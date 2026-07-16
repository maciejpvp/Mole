package client

import (
	"io"
	"net"
	"sync"
)

// BridgeTCP dials the local TCP target, then blindly splices all bytes between
// tunnelConn (the VPS tunnel leg) and the local service using io.Copy.
// Both connections are closed when either side EOF-s or errors.
// This function blocks until the bridge is fully torn down.
func BridgeTCP(tunnelConn net.Conn, localTarget string) {
	defer tunnelConn.Close()

	localConn, err := net.DialTimeout("tcp", localTarget, 10*secDur)
	if err != nil {
		logDebug("[tcp_bridge] dial local target %s: %v", localTarget, err)
		return
	}
	defer localConn.Close()

	logDebug("[tcp_bridge] bridging tunnel=%s <-> local=%s", tunnelConn.RemoteAddr(), localConn.RemoteAddr())

	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			tunnelConn.Close()
			localConn.Close()
		})
	}
	defer closeAll()

	done := make(chan struct{}, 2)

	// Try to use half-close for clean drain; fall back to full close.
	tunnelTCP, tunnelOK := tunnelConn.(*net.TCPConn)
	localTCP, localOK := localConn.(*net.TCPConn)

	if tunnelOK && localOK {
		go func() {
			n, err := io.Copy(localTCP, tunnelTCP)
			logDir("tunnel→local", n, err)
			localTCP.CloseWrite() //nolint:errcheck
			done <- struct{}{}
		}()
		go func() {
			n, err := io.Copy(tunnelTCP, localTCP)
			logDir("local→tunnel", n, err)
			tunnelTCP.CloseWrite() //nolint:errcheck
			done <- struct{}{}
		}()
	} else {
		go func() {
			n, err := io.Copy(localConn, tunnelConn)
			logDir("tunnel→local", n, err)
			closeAll()
			done <- struct{}{}
		}()
		go func() {
			n, err := io.Copy(tunnelConn, localConn)
			logDir("local→tunnel", n, err)
			closeAll()
			done <- struct{}{}
		}()
	}

	<-done
	<-done
	logDebug("[tcp_bridge] session closed: tunnel=%s local=%s", tunnelConn.RemoteAddr(), localConn.RemoteAddr())
}

func logDir(direction string, n int64, err error) {
	if err != nil && !isConnClosed(err) {
		logDebug("[tcp_bridge] %s: bytes=%d err=%v", direction, n, err)
	} else {
		logDebug("[tcp_bridge] %s: bytes=%d", direction, n)
	}
}
