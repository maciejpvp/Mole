package client

import (
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
