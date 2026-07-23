package client

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

const (
	signalNewConn  = byte(0x01)
	signalStartUDP = byte(0x02)
	roleControl    = byte(0x00)
	roleTCPLeg     = byte(0x01)
	roleUDPBridge  = byte(0x02)

	backoffBase = 1 * time.Second
	backoffMax  = 30 * time.Second
)

var Debug bool

func logDebug(format string, v ...any) {
	if Debug {
		log.Printf(format, v...)
	}
}

type Agent struct {
	serverAddr  string // e.g. "1.2.3.4:9000"
	localTarget string // e.g. "127.0.0.1:5173"
	localProto  string // "tcp" or "udp"
	token       string

	ctx      context.Context
	cancel   context.CancelFunc
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mu          sync.Mutex
	activeConns map[net.Conn]struct{}
}

func New(serverAddr, localTarget, localProto, token string) *Agent {
	ctx, cancel := context.WithCancel(context.Background())
	return &Agent{
		serverAddr:  serverAddr,
		localTarget: localTarget,
		localProto:  localProto,
		token:       token,
		ctx:         ctx,
		cancel:      cancel,
		stopCh:      make(chan struct{}),
		activeConns: make(map[net.Conn]struct{}),
	}
}

func (a *Agent) trackConn(c net.Conn) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.stopCh:
		c.Close()
		return false
	default:
		a.activeConns[c] = struct{}{}
		return true
	}
}

func (a *Agent) untrackConn(c net.Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.activeConns, c)
	c.Close()
}

func (a *Agent) Run() {
	backoff := backoffBase

	for {
		select {
		case <-a.stopCh:
			return
		default:
		}

		logDebug("[agent] connecting to server %s (proto=%s local=%s)", a.serverAddr, a.localProto, a.localTarget)

		dialCtx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		var dialer net.Dialer
		conn, err := dialer.DialContext(dialCtx, "tcp", a.serverAddr)
		cancel()

		if err != nil {
			select {
			case <-a.stopCh:
				return
			default:
			}
			logDebug("[agent] dial failed: %v — retrying in %s", err, backoff)
			a.sleep(backoff)
			backoff = min(backoff*2, backoffMax)
			continue
		}

		if !a.trackConn(conn) {
			return
		}

		logDebug("[agent] control connection established: local=%s remote=%s", conn.LocalAddr(), conn.RemoteAddr())
		backoff = backoffBase

		if err := a.sendHandshake(conn, roleControl); err != nil {
			select {
			case <-a.stopCh:
				a.untrackConn(conn)
				return
			default:
			}
			logDebug("[agent] token handshake failed: %v — reconnecting", err)
			a.untrackConn(conn)
			continue
		}

		a.readSignals(conn)
		a.untrackConn(conn)

		select {
		case <-a.stopCh:
			return
		default:
			logDebug("[agent] control connection lost — reconnecting")
		}
	}
}

// Stop signals the agent to stop reconnecting, closes all active connections, and exits Run.
func (a *Agent) Stop() {
	a.stopOnce.Do(func() {
		a.cancel()
		close(a.stopCh)
		a.mu.Lock()
		for c := range a.activeConns {
			c.Close()
		}
		a.activeConns = make(map[net.Conn]struct{})
		a.mu.Unlock()
	})
}

// Wait blocks until all in-flight bridge goroutines have finished.
func (a *Agent) Wait() {
	a.wg.Wait()
}

// sendHandshake identifies the tunnel and connection role. Wire format:
// 4-byte token length, token bytes, then a one-byte connection role.
func (a *Agent) sendHandshake(conn net.Conn, role byte) error {
	tokenBytes := []byte(a.token)
	hdr := make([]byte, 4)
	putUint32BE(hdr, uint32(len(tokenBytes)))
	if _, err := conn.Write(hdr); err != nil {
		return err
	}
	if _, err := conn.Write(tokenBytes); err != nil {
		return err
	}
	_, err := conn.Write([]byte{role})
	return err
}

func putUint32BE(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func (a *Agent) readSignals(control net.Conn) {
	buf := make([]byte, 1)
	for {
		_, err := control.Read(buf)
		if err != nil {
			select {
			case <-a.stopCh:
				logDebug("[agent] shutting down signal reader")
			default:
				logDebug("[agent] control read error: %v", err)
			}
			return
		}

		switch buf[0] {
		case signalNewConn:
			logDebug("[agent] server signalled new %s session", a.localProto)
			a.dispatchTCPBridge()
		case signalStartUDP:
			logDebug("[agent] server requested UDP bridge")
			a.dispatchUDPBridge()
		default:
			// Unknown signal bytes are silently discarded — keeps the relay blind.
			logDebug("[agent] unknown signal byte 0x%02x — ignored", buf[0])
		}
	}
}

func (a *Agent) dispatchTCPBridge() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		dialCtx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		var dialer net.Dialer
		tunnelConn, err := dialer.DialContext(dialCtx, "tcp", a.serverAddr)
		cancel()
		if err != nil {
			logDebug("[agent] tunnel dial error: %v", err)
			return
		}
		if !a.trackConn(tunnelConn) {
			return
		}
		defer a.untrackConn(tunnelConn)

		if err := a.sendHandshake(tunnelConn, roleTCPLeg); err != nil {
			logDebug("[agent] TCP tunnel handshake error: %v", err)
			return
		}
		logDebug("[agent] tunnel leg opened: local=%s remote=%s", tunnelConn.LocalAddr(), tunnelConn.RemoteAddr())

		BridgeTCP(tunnelConn, a.localTarget)
	}()
}

func (a *Agent) dispatchUDPBridge() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		dialCtx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		var dialer net.Dialer
		tunnelConn, err := dialer.DialContext(dialCtx, "tcp", a.serverAddr)
		cancel()
		if err != nil {
			logDebug("[agent] UDP bridge dial error: %v", err)
			return
		}
		if !a.trackConn(tunnelConn) {
			return
		}
		defer a.untrackConn(tunnelConn)

		if err := a.sendHandshake(tunnelConn, roleUDPBridge); err != nil {
			logDebug("[agent] UDP bridge handshake error: %v", err)
			return
		}
		BridgeUDP(tunnelConn, a.localTarget, a.stopCh)
	}()
}

func (a *Agent) sleep(d time.Duration) {
	select {
	case <-time.After(d):
	case <-a.stopCh:
	}
}
