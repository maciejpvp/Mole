package orchestrator

import (
	"net"
	"testing"
)

func TestProvisionAndDeprovision(t *testing.T) {
	port := freeTCPPort(t)
	engine, err := New(Config{ControlPort: 9000, PortMin: port, PortMax: port, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	response, err := engine.Provision(ProvisionRequest{
		TunnelID: "tunnel-1",
		UserID:   "user-1",
		Protocol: "tcp",
		Token:    "a-long-unpredictable-test-token",
	})
	if err != nil {
		t.Fatalf("provision tunnel: %v", err)
	}
	if response.OutboundPort != port || response.PublicHost != "tunnels.example.test" || response.ControlPort != 9000 {
		t.Fatalf("unexpected provision response: %+v", response)
	}
	if err := engine.Deprovision("tunnel-1"); err != nil {
		t.Fatalf("deprovision tunnel: %v", err)
	}
}

func TestUDPFrameRoundTrip(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	go func() {
		_, _ = left.Write(encodeUDPFrame("127.0.0.1:1234", []byte("payload")))
	}()
	address, payload, err := readUDPFrame(right)
	if err != nil {
		t.Fatalf("read UDP frame: %v", err)
	}
	if address != "127.0.0.1:1234" || string(payload) != "payload" {
		t.Fatalf("unexpected UDP frame: %q, %q", address, payload)
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
