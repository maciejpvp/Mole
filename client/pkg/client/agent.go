package client

import (
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

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func New(serverAddr, localTarget, localProto, token string) *Agent {
	return &Agent{
		serverAddr:  serverAddr,
		localTarget: localTarget,
		localProto:  localProto,
		token:       token,
		stopCh:      make(chan struct{}),
	}
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

		conn, err := net.DialTimeout("tcp", a.serverAddr, 10*time.Second)
		if err != nil {
			logDebug("[agent] dial failed: %v — retrying in %s", err, backoff)
			a.sleep(backoff)
			backoff = min(backoff*2, backoffMax)
			continue
		}

		logDebug("[agent] control connection established: local=%s remote=%s", conn.LocalAddr(), conn.RemoteAddr())
		backoff = backoffBase

		if err := a.sendHandshake(conn, roleControl); err != nil {
			logDebug("[agent] token handshake failed: %v — reconnecting", err)
			conn.Close()
			continue
		}

		a.readSignals(conn)

		logDebug("[agent] control connection lost — reconnecting")
		conn.Close()
	}
}

// Stop signals the agent to stop reconnecting and exit Run.
func (a *Agent) Stop() {
	a.stopOnce.Do(func() {
		close(a.stopCh)
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
		tunnelConn, err := net.DialTimeout("tcp", a.serverAddr, 10*time.Second)
		if err != nil {
			logDebug("[agent] tunnel dial error: %v", err)
			return
		}
		if err := a.sendHandshake(tunnelConn, roleTCPLeg); err != nil {
			logDebug("[agent] TCP tunnel handshake error: %v", err)
			tunnelConn.Close()
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
		tunnelConn, err := net.DialTimeout("tcp", a.serverAddr, 10*time.Second)
		if err != nil {
			logDebug("[agent] UDP bridge dial error: %v", err)
			return
		}
		if err := a.sendHandshake(tunnelConn, roleUDPBridge); err != nil {
			logDebug("[agent] UDP bridge handshake error: %v", err)
			tunnelConn.Close()
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
