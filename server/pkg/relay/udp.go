package relay

import (
	"log"
	"net"
	"sync"
	"time"
)

const (
	// udpSessionTimeout is how long a UDP "session" (keyed by remote address)
	// is kept alive without receiving a packet before it is evicted.
	udpSessionTimeout = 5 * time.Minute

	// udpMaxPacketSize is the maximum UDP datagram we will buffer.
	udpMaxPacketSize = 65535
)

// udpSession tracks one logical UDP client session identified by the remote
// public address. Packets destined for the tunnel are queued on ch.
type udpSession struct {
	ch       chan []byte
	lastSeen time.Time
	mu       sync.Mutex
}

func (s *udpSession) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

func (s *udpSession) idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastSeen) > udpSessionTimeout
}

type UDPRelay struct {
	publicConn *net.UDPConn
	sessions   sync.Map // map[string]*udpSession
	stopOnce   sync.Once
	stopCh     chan struct{}
}

// NewUDPRelay constructs a UDPRelay bound to publicConn.
func NewUDPRelay(publicConn *net.UDPConn) *UDPRelay {
	r := &UDPRelay{
		publicConn: publicConn,
		stopCh:     make(chan struct{}),
	}
	return r
}

func (r *UDPRelay) RunInbound() <-chan []byte {
	out := make(chan []byte, 256)

	go r.gcSessions()

	go func() {
		defer close(out)
		buf := make([]byte, udpMaxPacketSize)
		for {
			n, remote, err := r.publicConn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-r.stopCh:
					return
				default:
					if !isClosedErr(err) {
						log.Printf("[relay/udp] inbound read error: %v", err)
					}
					return
				}
			}

			payload := make([]byte, n)
			copy(payload, buf[:n])

			addrStr := remote.String()
			frame := encodeFrame(addrStr, payload)

			// Ensure a session exists so we can send replies back.
			r.ensureSession(addrStr)

			select {
			case out <- frame:
			case <-r.stopCh:
				return
			}
		}
	}()

	return out
}

func (r *UDPRelay) RunOutbound(in <-chan []byte) {
	go func() {
		for {
			select {
			case frame, ok := <-in:
				if !ok {
					return
				}
				addrStr, payload, err := decodeFrame(frame)
				if err != nil {
					log.Printf("[relay/udp] outbound decode error: %v", err)
					continue
				}
				remote, err := net.ResolveUDPAddr("udp", addrStr)
				if err != nil {
					log.Printf("[relay/udp] resolve %q: %v", addrStr, err)
					continue
				}
				if _, err := r.publicConn.WriteToUDP(payload, remote); err != nil && !isClosedErr(err) {
					log.Printf("[relay/udp] write to %s: %v", addrStr, err)
				}
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *UDPRelay) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		r.publicConn.Close()
	})
}

func (r *UDPRelay) ensureSession(addrStr string) {
	val, loaded := r.sessions.LoadOrStore(addrStr, &udpSession{
		ch:       make(chan []byte, 64),
		lastSeen: time.Now(),
	})
	if loaded {
		val.(*udpSession).touch()
	}
}

func (r *UDPRelay) gcSessions() {
	ticker := time.NewTicker(udpSessionTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.sessions.Range(func(key, val any) bool {
				s := val.(*udpSession)
				if s.idle() {
					log.Printf("[relay/udp] evicting idle session: addr=%s", key)
					r.sessions.Delete(key)
				}
				return true
			})
		case <-r.stopCh:
			return
		}
	}
}

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

// decodeFrame is the inverse of encodeFrame.
func decodeFrame(frame []byte) (addr string, payload []byte, err error) {
	if len(frame) < 4 {
		return "", nil, net.UnknownNetworkError("frame too short for addr len")
	}
	addrLen := int(getUint32(frame))
	if len(frame) < 4+addrLen+4 {
		return "", nil, net.UnknownNetworkError("frame too short for addr")
	}
	addr = string(frame[4 : 4+addrLen])
	payLen := int(getUint32(frame[4+addrLen:]))
	start := 4 + addrLen + 4
	if len(frame) < start+payLen {
		return "", nil, net.UnknownNetworkError("frame too short for payload")
	}
	payload = frame[start : start+payLen]
	return addr, payload, nil
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
