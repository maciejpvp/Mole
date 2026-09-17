package client

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestAgentStop(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	ag := New(listener.Addr().String(), "127.0.0.1:8080", "tcp", "test-token")

	runDone := make(chan struct{})
	go func() {
		ag.Run()
		close(runDone)
	}()

	time.Sleep(100 * time.Millisecond)

	ag.Stop()

	select {
	case <-runDone:
		// Success: Run exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("agent.Run() failed to exit after agent.Stop()")
	}

	waitDone := make(chan struct{})
	go func() {
		ag.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// Success: Wait exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("agent.Wait() failed to exit after agent.Stop()")
	}
}

// TestAgentStopsOnAuthRejection ensures a refused token ends the agent instead
// of leaving it reconnecting forever against a tunnel that no longer exists.
func TestAgentStopsOnAuthRejection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	attempts := make(chan struct{}, 8)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			select {
			case attempts <- struct{}{}:
			default:
			}
			go func(c net.Conn) {
				defer c.Close()
				// Read the handshake, then refuse it.
				header := make([]byte, 4)
				if _, err := io.ReadFull(c, header); err != nil {
					return
				}
				length := int(uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3]))
				if _, err := io.ReadFull(c, make([]byte, length+1)); err != nil {
					return
				}
				_, _ = c.Write([]byte{signalAuthRejected})
			}(conn)
		}
	}()

	ag := New(listener.Addr().String(), "127.0.0.1:8080", "tcp", "test-token")
	runDone := make(chan struct{})
	go func() {
		ag.Run()
		close(runDone)
	}()

	select {
	case <-runDone:
	case <-time.After(3 * time.Second):
		t.Fatal("agent kept reconnecting after its token was rejected")
	}
	if len(attempts) != 1 {
		t.Fatalf("expected exactly one connection attempt, got %d", len(attempts))
	}
}
