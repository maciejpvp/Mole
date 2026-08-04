package orchestrator

import (
	"errors"
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

func TestConnectionStatusUpdates(t *testing.T) {
	engine, err := New(Config{ControlPort: 9000, PortMin: 10000, PortMax: 10000, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	engine.recordConnectionStatus("tunnel-1", "active")
	update := <-engine.ConnectionStatusUpdates()
	if update.TunnelID != "tunnel-1" || update.Status != "active" {
		t.Fatalf("unexpected connection status update: %+v", update)
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

func TestGlobalTransferFusePersistsAndStops(t *testing.T) {
	stateDir := t.TempDir()
	engine, err := New(Config{
		ControlPort:              9000,
		PortMin:                  10000,
		PortMax:                  10000,
		PublicHost:               "tunnels.example.test",
		GlobalTransferLimitBytes: 10,
		GlobalTransferStateDir:   stateDir,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	item := &tunnel{engine: engine, id: "tunnel-1", userID: "user-1", sessions: make(map[net.Conn]struct{})}
	engine.mu.Lock()
	engine.tunnels[item.id] = item
	engine.users[item.userID] = &userUsage{tunnels: map[string]*tunnel{item.id: item}}
	engine.mu.Unlock()

	engine.recordBytes(item.id, 10)
	select {
	case <-engine.stopCh:
	default:
		t.Fatal("expected global fuse to stop the engine")
	}

	if _, err := New(Config{
		ControlPort:              9000,
		PortMin:                  10000,
		PortMax:                  10000,
		PublicHost:               "tunnels.example.test",
		GlobalTransferLimitBytes: 10,
		GlobalTransferStateDir:   stateDir,
	}); !errors.Is(err, ErrGlobalFuseTripped) {
		t.Fatalf("expected persisted fuse to reject restart, got %v", err)
	}
}

func TestZeroGlobalTransferLimitIsUnlimited(t *testing.T) {
	engine, err := New(Config{
		ControlPort:              9000,
		PortMin:                  10000,
		PortMax:                  10000,
		PublicHost:               "tunnels.example.test",
		GlobalTransferLimitBytes: 0,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	item := &tunnel{engine: engine, id: "tunnel-unlimited", userID: "user-1", sessions: make(map[net.Conn]struct{})}
	engine.mu.Lock()
	engine.tunnels[item.id] = item
	engine.users[item.userID] = &userUsage{tunnels: map[string]*tunnel{item.id: item}}
	engine.mu.Unlock()

	engine.recordBytes(item.id, 1<<62)
	select {
	case <-engine.stopCh:
		t.Fatal("zero global transfer limit must not stop the engine")
	default:
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
