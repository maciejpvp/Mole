package client

import (
	"net"
	"sync"
	"time"
)

const (
	udpIdleTimeout   = 5 * time.Minute
	udpMaxPacketSize = 65535

	// secDur is used by tcp_bridge.go as well — kept here as the shared
	// package-level constant for dial timeouts.
	secDur = time.Second
)

// udpSession tracks one logical UDP session keyed by the remote address string
// that the server embedded in the frame (i.e. the original public client IP:port).
type udpSession struct {
	localConn *net.UDPConn // connected to the local target (e.g. WireGuard)
	lastSeen  time.Time
	mu        sync.Mutex
}

func (s *udpSession) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

func (s *udpSession) idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastSeen) > udpIdleTimeout
}

// BridgeUDP reads framed UDP packets from tunnelConn (a TCP connection to the
// VPS server carrying the framed UDP protocol) and forwards each datagram to
// the local UDP target. Replies from the local target are framed and written
// back through tunnelConn to the VPS.
//
// Wire frame format (matches server/pkg/relay/udp.go):
//
//	[4B addr-len][addr string][4B payload-len][payload bytes]
//
// BridgeUDP blocks until tunnelConn is closed or stopCh is closed.
func BridgeUDP(tunnelConn net.Conn, localTarget string, stopCh <-chan struct{}) {
	defer tunnelConn.Close()

	localUDPAddr, err := net.ResolveUDPAddr("udp", localTarget)
	if err != nil {
		logDebug("[udp_bridge] resolve local target %s: %v", localTarget, err)
		return
	}

	var sessions sync.Map // map[string]*udpSession

	// Start GC for idle sessions.
	gcStop := make(chan struct{})
	go gcSessions(&sessions, gcStop)
	defer close(gcStop)

	logDebug("[udp_bridge] ready: tunnel=%s → local=%s", tunnelConn.RemoteAddr(), localTarget)

	hdr := make([]byte, 4)
	for {
		// --- Read one frame from the VPS tunnel ---

		// 1. addr length
		if err := readFullConn(tunnelConn, hdr); err != nil {
			if !isConnClosed(err) {
				logDebug("[udp_bridge] read addr-len: %v", err)
			}
			return
		}
		addrLen := int(getUint32(hdr))

		// 2. addr bytes
		addrBuf := make([]byte, addrLen)
		if err := readFullConn(tunnelConn, addrBuf); err != nil {
			logDebug("[udp_bridge] read addr: %v", err)
			return
		}
		remoteAddrStr := string(addrBuf)

		// 3. payload length
		if err := readFullConn(tunnelConn, hdr); err != nil {
			logDebug("[udp_bridge] read payload-len: %v", err)
			return
		}
		payLen := int(getUint32(hdr))

		// 4. payload
		payload := make([]byte, payLen)
		if err := readFullConn(tunnelConn, payload); err != nil {
			logDebug("[udp_bridge] read payload: %v", err)
			return
		}

		// --- Dispatch to local UDP socket ---

		sessVal, loaded := sessions.Load(remoteAddrStr)
		if !loaded {
			// Create a new UDP socket for this logical session.
			lconn, err := net.DialUDP("udp", nil, localUDPAddr)
			if err != nil {
				logDebug("[udp_bridge] dial local UDP %s: %v", localTarget, err)
				continue
			}
			sess := &udpSession{localConn: lconn, lastSeen: time.Now()}
			sessions.Store(remoteAddrStr, sess)
			sessVal = sess
			logDebug("[udp_bridge] new session: remote=%s → local=%s", remoteAddrStr, localTarget)

			// Read replies from the local service and forward them back.
			go readLocalUDP(lconn, remoteAddrStr, tunnelConn, stopCh)
		}

		sess := sessVal.(*udpSession)
		sess.touch()

		if _, err := sess.localConn.Write(payload); err != nil && !isConnClosed(err) {
			logDebug("[udp_bridge] write to local UDP: %v", err)
		}
	}
}

// readLocalUDP reads datagrams from localConn and forwards them as framed
// packets back through tunnelConn to the VPS server.
func readLocalUDP(localConn *net.UDPConn, remoteAddrStr string, tunnelConn net.Conn, stopCh <-chan struct{}) {
	defer localConn.Close()

	buf := make([]byte, udpMaxPacketSize)
	for {
		localConn.SetReadDeadline(time.Now().Add(udpIdleTimeout)) //nolint:errcheck
		n, err := localConn.Read(buf)
		if err != nil {
			select {
			case <-stopCh:
			default:
				if !isConnClosed(err) {
					logDebug("[udp_bridge] local read error (remote=%s): %v", remoteAddrStr, err)
				}
			}
			return
		}

		frame := encodeUDPFrame(remoteAddrStr, buf[:n])
		if _, err := tunnelConn.Write(frame); err != nil && !isConnClosed(err) {
			logDebug("[udp_bridge] write to tunnel (remote=%s): %v", remoteAddrStr, err)
			return
		}
	}
}

// gcSessions evicts idle UDP sessions periodically.
func gcSessions(sessions *sync.Map, stop <-chan struct{}) {
	ticker := time.NewTicker(udpIdleTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sessions.Range(func(key, val any) bool {
				s := val.(*udpSession)
				if s.idle() {
					logDebug("[udp_bridge] evicting idle session: %s", key)
					s.localConn.Close()
					sessions.Delete(key)
				}
				return true
			})
		case <-stop:
			return
		}
	}
}

// encodeUDPFrame serialises (addr, payload) into the shared wire format.
func encodeUDPFrame(addr string, payload []byte) []byte {
	ab := []byte(addr)
	buf := make([]byte, 4+len(ab)+4+len(payload))
	i := 0
	putUint32(buf[i:], uint32(len(ab)))
	i += 4
	copy(buf[i:], ab)
	i += len(ab)
	putUint32(buf[i:], uint32(len(payload)))
	i += 4
	copy(buf[i:], payload)
	return buf
}

// readFullConn reads exactly len(buf) bytes from conn.
func readFullConn(conn net.Conn, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return err
		}
	}
	return nil
}

// isConnClosed returns true for the sentinel "use of closed network connection"
// error so we can suppress it from noisy logs.
func isConnClosed(err error) bool {
	if err == nil {
		return false
	}
	const closed = "use of closed network connection"
	s := err.Error()
	for i := 0; i <= len(s)-len(closed); i++ {
		if s[i:i+len(closed)] == closed {
			return true
		}
	}
	return false
}

func putUint32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func getUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
