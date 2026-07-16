package relay

import (
	"io"
	"log"
	"net"
	"sync"
)

func TCP(left, right net.Conn) {
	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			left.Close()
			right.Close()
		})
	}
	defer closeAll()

	done := make(chan struct{}, 2)

	copy := func(dst, src net.Conn, label string) {
		n, err := io.Copy(dst, src)
		if err != nil && !isClosedErr(err) {
			log.Printf("[relay/tcp] %s: copy ended: bytes=%d err=%v", label, n, err)
		} else {
			log.Printf("[relay/tcp] %s: copy ended: bytes=%d", label, n)
		}
		dst.(*net.TCPConn).CloseWrite() //nolint:errcheck
		done <- struct{}{}
	}

	leftTCP, leftOK := left.(*net.TCPConn)
	rightTCP, rightOK := right.(*net.TCPConn)

	if leftOK && rightOK {
		go func() {
			n, err := io.Copy(rightTCP, leftTCP)
			logCopy("left→right", n, err)
			rightTCP.CloseWrite() //nolint:errcheck
			done <- struct{}{}
		}()
		go func() {
			n, err := io.Copy(leftTCP, rightTCP)
			logCopy("right→left", n, err)
			leftTCP.CloseWrite() //nolint:errcheck
			done <- struct{}{}
		}()
	} else {
		_ = copy
		go func() {
			n, err := io.Copy(right, left)
			logCopy("left→right", n, err)
			closeAll()
			done <- struct{}{}
		}()
		go func() {
			n, err := io.Copy(left, right)
			logCopy("right→left", n, err)
			closeAll()
			done <- struct{}{}
		}()
	}

	<-done
	<-done
}

func logCopy(direction string, n int64, err error) {
	if err != nil && !isClosedErr(err) {
		log.Printf("[relay/tcp] %s: bytes=%d err=%v", direction, n, err)
	} else {
		log.Printf("[relay/tcp] %s: bytes=%d", direction, n)
	}
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	const closed = "use of closed network connection"
	return contains(err.Error(), closed)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
