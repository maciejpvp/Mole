package orchestrator

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"Mole/server/pkg/relay"
)

const (
	dialTimeout   = 10 * time.Second
	signalNewConn = byte(0x01)
)

type Engine struct {
	controlPort int
	publicPort  int
	secret      string

	pendingTunnels chan net.Conn

	controlConn net.Conn
	controlMu   sync.RWMutex

	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a new Engine bound to the given ports.
// secret is the shared token clients must present before being registered.
func New(controlPort, publicPort int, secret string) *Engine {
	return &Engine{
		controlPort:    controlPort,
		publicPort:     publicPort,
		secret:         secret,
		pendingTunnels: make(chan net.Conn, 64),
		stopCh:         make(chan struct{}),
	}
}

// Run starts all listeners and blocks until Stop is called.
func (e *Engine) Run() error {
	controlAddr := fmt.Sprintf(":%d", e.controlPort)
	publicTCPAddr := fmt.Sprintf(":%d", e.publicPort)
	publicUDPAddr := fmt.Sprintf(":%d", e.publicPort)

	controlLn, err := net.Listen("tcp", controlAddr)
	if err != nil {
		return fmt.Errorf("control listener: %w", err)
	}
	defer controlLn.Close()
	log.Printf("[orchestrator] control listener up on %s", controlAddr)

	publicTCPLn, err := net.Listen("tcp", publicTCPAddr)
	if err != nil {
		return fmt.Errorf("public TCP listener: %w", err)
	}
	defer publicTCPLn.Close()
	log.Printf("[orchestrator] public TCP listener up on %s", publicTCPAddr)

	publicUDPConn, err := net.ListenPacket("udp", publicUDPAddr)
	if err != nil {
		return fmt.Errorf("public UDP listener: %w", err)
	}
	defer publicUDPConn.Close()
	log.Printf("[orchestrator] public UDP listener up on %s", publicUDPAddr)

	// Shut down all listeners when Stop is called.
	go func() {
		<-e.stopCh
		controlLn.Close()
		publicTCPLn.Close()
		publicUDPConn.Close()
	}()

	go e.acceptControl(controlLn)
	go e.acceptPublicTCP(publicTCPLn)
	go e.handlePublicUDP(publicUDPConn.(*net.UDPConn))

	<-e.stopCh
	return nil
}

func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
	})
}

func (e *Engine) acceptControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-e.stopCh:
				return
			default:
				log.Printf("[orchestrator] control accept error: %v", err)
				continue
			}
		}

		e.controlMu.RLock()
		hasControl := e.controlConn != nil
		e.controlMu.RUnlock()

		if !hasControl {
			e.registerControl(conn)
		} else {
			// This is a tunnel leg — queue it for an awaiting public connection.
			log.Printf("[orchestrator] tunnel leg registered: remote=%s", conn.RemoteAddr())
			select {
			case e.pendingTunnels <- conn:
			case <-e.stopCh:
				conn.Close()
			}
		}
	}
}

// registerControl reads the client's secret handshake, validates it, and
// then stores the control connection and starts monitoring it.
// If the secret is wrong the connection is rejected immediately.
// If the client disconnects, the control connection is cleared so the next
// dial is treated as a fresh registration.
func (e *Engine) registerControl(conn net.Conn) {
	// --- secret handshake ---
	// Wire format: 4-byte big-endian length prefix followed by the secret bytes.
	hdr := make([]byte, 4)
	if _, err := readFull(conn, hdr); err != nil {
		log.Printf("[orchestrator] handshake read error from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	secretLen := int(getUint32(hdr))
	if secretLen > 4096 {
		log.Printf("[orchestrator] handshake secret too long (%d) from %s — rejected", secretLen, conn.RemoteAddr())
		conn.Close()
		return
	}
	secretBuf := make([]byte, secretLen)
	if _, err := readFull(conn, secretBuf); err != nil {
		log.Printf("[orchestrator] handshake secret read error from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	if string(secretBuf) != e.secret {
		log.Printf("[orchestrator] invalid secret from %s — rejected", conn.RemoteAddr())
		conn.Close()
		return
	}
	// --- end handshake ---

	e.controlMu.Lock()
	e.controlConn = conn
	e.controlMu.Unlock()

	log.Printf("[orchestrator] client registered: remote=%s", conn.RemoteAddr())

	// Monitor for control connection close.
	go func() {
		buf := make([]byte, 1)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				log.Printf("[orchestrator] control connection lost: %v", err)
				e.controlMu.Lock()
				if e.controlConn == conn {
					e.controlConn = nil
				}
				e.controlMu.Unlock()
				conn.Close()
				return
			}
			// The client may send keep-alive bytes; we discard them.
		}
	}()
}

func (e *Engine) acceptPublicTCP(ln net.Listener) {
	for {
		publicConn, err := ln.Accept()
		if err != nil {
			select {
			case <-e.stopCh:
				return
			default:
				log.Printf("[orchestrator] public TCP accept error: %v", err)
				continue
			}
		}

		log.Printf("[orchestrator] public TCP connection: remote=%s", publicConn.RemoteAddr())
		go e.bridgePublicTCP(publicConn)
	}
}

func (e *Engine) bridgePublicTCP(publicConn net.Conn) {
	defer func() {
		if publicConn != nil {
			publicConn.Close()
		}
	}()

	if err := e.signal(signalNewConn); err != nil {
		log.Printf("[orchestrator] failed to signal client: %v", err)
		return
	}
	select {
	case tunnelConn := <-e.pendingTunnels:
		log.Printf("[orchestrator] splicing public=%s <-> tunnel=%s",
			publicConn.RemoteAddr(), tunnelConn.RemoteAddr())
		pc := publicConn
		publicConn = nil // prevent defer double-close
		relay.TCP(pc, tunnelConn)

	case <-time.After(dialTimeout):
		log.Printf("[orchestrator] timed out waiting for tunnel leg for %s", publicConn.RemoteAddr())

	case <-e.stopCh:
	}
}

// handlePublicUDP forwards UDP datagrams between the public internet and the
// control connection using the framed wire protocol from relay/udp.go.
func (e *Engine) handlePublicUDP(conn *net.UDPConn) {
	udpRelay := relay.NewUDPRelay(conn)

	inbound := udpRelay.RunInbound()

	// Outbound: frames coming from the client over TCP control connection.
	// We read them in a goroutine and push them into a channel for RunOutbound.
	outboundCh := make(chan []byte, 256)
	udpRelay.RunOutbound(outboundCh)

	// Forward inbound UDP frames to the client over the control connection.
	go func() {
		for frame := range inbound {
			e.controlMu.RLock()
			cc := e.controlConn
			e.controlMu.RUnlock()

			if cc == nil {
				log.Printf("[orchestrator] UDP frame dropped — no client connected")
				continue
			}
			if _, err := cc.Write(frame); err != nil && !isClosedErr(err) {
				log.Printf("[orchestrator] UDP forward to client error: %v", err)
			}
		}
	}()

	// Read UDP reply frames from the client over the control connection and
	// push them to the outbound channel.
	go func() {
		defer close(outboundCh)
		hdr := make([]byte, 4)
		for {
			e.controlMu.RLock()
			cc := e.controlConn
			e.controlMu.RUnlock()

			if cc == nil {
				// Back-off and retry until a client connects.
				select {
				case <-e.stopCh:
					return
				case <-time.After(500 * time.Millisecond):
					continue
				}
			}

			// Read the 4-byte addr-length prefix.
			if _, err := readFull(cc, hdr); err != nil {
				if !isClosedErr(err) {
					log.Printf("[orchestrator] UDP reply read error: %v", err)
				}
				continue
			}
			addrLen := int(getUint32(hdr))
			addrBuf := make([]byte, addrLen)
			if _, err := readFull(cc, addrBuf); err != nil {
				log.Printf("[orchestrator] UDP reply addr read error: %v", err)
				continue
			}
			if _, err := readFull(cc, hdr); err != nil {
				log.Printf("[orchestrator] UDP reply payload-len read error: %v", err)
				continue
			}
			payLen := int(getUint32(hdr))
			payBuf := make([]byte, payLen)
			if _, err := readFull(cc, payBuf); err != nil {
				log.Printf("[orchestrator] UDP reply payload read error: %v", err)
				continue
			}

			frame := encodeFrame(string(addrBuf), payBuf)
			select {
			case outboundCh <- frame:
			case <-e.stopCh:
				return
			}
		}
	}()

	<-e.stopCh
	udpRelay.Stop()
}

// signal writes a single-byte command to the active control connection.
func (e *Engine) signal(cmd byte) error {
	e.controlMu.RLock()
	cc := e.controlConn
	e.controlMu.RUnlock()

	if cc == nil {
		return fmt.Errorf("no client connected")
	}
	_, err := cc.Write([]byte{cmd})
	return err
}

// isClosedErr returns true for the "use of closed network connection" sentinel.
func isClosedErr(err error) bool {
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

// readFull reads exactly len(buf) bytes from conn, returning an error if
// fewer bytes are available before EOF.
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// encodeFrame mirrors relay.encodeFrame to avoid a circular import.
func encodeFrame(addr string, payload []byte) []byte {
	addrBytes := []byte(addr)
	buf := make([]byte, 4+len(addrBytes)+4+len(payload))
	i := 0
	putUint32(buf[i:], uint32(len(addrBytes)))
	i += 4
	copy(buf[i:], addrBytes)
	i += len(addrBytes)
	putUint32(buf[i:], uint32(len(payload)))
	i += 4
	copy(buf[i:], payload)
	return buf
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
