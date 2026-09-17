package relay

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
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

func TestStartDeliversConnectionStatusToCallback(t *testing.T) {
	port := freeTCPPort(t)
	engine, err := New(Config{ControlPort: port, PortMin: port + 1, PortMax: port + 1, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	statuses := make(chan ConnectionStatusUpdate, 1)
	engine.SetCallbacks(Callbacks{SetConnectionStatus: func(_ context.Context, update ConnectionStatusUpdate) error {
		statuses <- update
		return nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer engine.Stop()
	engine.recordConnectionStatus("tunnel-1", "active")
	select {
	case update := <-statuses:
		if update.TunnelID != "tunnel-1" || update.Status != "active" {
			t.Fatalf("unexpected status update: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for direct status callback")
	}
}

// writeHandshake mirrors the client's wire format: 4-byte token length, the
// token, then the connection role.
func writeHandshake(t *testing.T, conn net.Conn, token string, role byte) {
	t.Helper()
	payload := []byte(token)
	header := []byte{byte(len(payload) >> 24), byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
	if _, err := conn.Write(append(append(header, payload...), role)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
}

// TestRestoredTunnelAcceptsItsOriginalToken covers the restart bug end to end:
// a tunnel rebuilt from persisted state must authenticate the same token the
// client already has.
func TestRestoredTunnelAcceptsItsOriginalToken(t *testing.T) {
	const token = "a-long-unpredictable-test-token"
	controlPort := freeTCPPort(t)
	publicPort := freeTCPPort(t)
	engine, err := New(Config{ControlPort: controlPort, PortMin: publicPort, PortMax: publicPort, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	response, err := engine.Restore(RestoreRequest{
		TunnelID:     "tunnel-1",
		UserID:       "user-1",
		Protocol:     "tcp",
		TokenHash:    sha256.Sum256([]byte(token)),
		OutboundPort: publicPort,
	})
	if err != nil {
		t.Fatalf("restore tunnel: %v", err)
	}
	if response.OutboundPort != publicPort || response.Rebound {
		t.Fatalf("expected the original public port to be reclaimed, got %+v", response)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer engine.Stop()

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)))
	if err != nil {
		t.Fatalf("dial control port: %v", err)
	}
	defer conn.Close()
	writeHandshake(t, conn, token, roleControl)

	select {
	case update := <-engine.ConnectionStatusUpdates():
		if update.TunnelID != "tunnel-1" || update.Status != "active" {
			t.Fatalf("unexpected status update: %+v", update)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restored tunnel did not accept its original token")
	}
}

func TestRestoreRebindsWhenOriginalPortIsUnavailable(t *testing.T) {
	first := freeTCPPort(t)
	second := freeTCPPort(t)
	if second == first {
		t.Skip("could not reserve two distinct ports")
	}
	engine, err := New(Config{ControlPort: 9000, PortMin: min(first, second), PortMax: max(first, second), PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	// Squat the port the tunnel expects to reclaim.
	squatter, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(first)))
	if err != nil {
		t.Fatalf("squat port: %v", err)
	}
	defer squatter.Close()

	response, err := engine.Restore(RestoreRequest{
		TunnelID: "tunnel-1", UserID: "user-1", Protocol: "tcp",
		TokenHash: sha256.Sum256([]byte("token")), OutboundPort: first,
	})
	if err != nil {
		t.Fatalf("restore tunnel: %v", err)
	}
	if !response.Rebound {
		t.Fatal("expected Rebound to report that the original port was unavailable")
	}
	if response.OutboundPort == first {
		t.Fatalf("expected a different public port, got %d", response.OutboundPort)
	}
	defer engine.Deprovision("tunnel-1")
}

func TestRestoreRejectsDuplicatesAndTrippedFuse(t *testing.T) {
	port := freeTCPPort(t)
	engine, err := New(Config{ControlPort: 9000, PortMin: port, PortMax: port, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	request := RestoreRequest{
		TunnelID: "tunnel-1", UserID: "user-1", Protocol: "tcp",
		TokenHash: sha256.Sum256([]byte("token")), OutboundPort: port,
	}
	if _, err := engine.Restore(request); err != nil {
		t.Fatalf("restore tunnel: %v", err)
	}
	if _, err := engine.Restore(request); err == nil {
		t.Fatal("expected the second restore of the same tunnel to fail")
	}
	engine.Deprovision("tunnel-1")

	engine.mu.Lock()
	engine.globalFuseTripped = true
	engine.mu.Unlock()
	if _, err := engine.Restore(request); !errors.Is(err, ErrGlobalFuseTripped) {
		t.Fatalf("expected ErrGlobalFuseTripped, got %v", err)
	}
}

func TestRestoreRejectsEmptyTokenHash(t *testing.T) {
	port := freeTCPPort(t)
	engine, err := New(Config{ControlPort: 9000, PortMin: port, PortMax: port, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	if _, err := engine.Restore(RestoreRequest{
		TunnelID: "tunnel-1", UserID: "user-1", Protocol: "tcp", OutboundPort: port,
	}); err == nil {
		t.Fatal("expected an empty token hash to be rejected")
	}
}

// TestRejectedHandshakeSignalsOnlyTheControlConnection covers both halves of
// the rejection contract: the control connection learns its token was refused,
// while a data leg is closed silently so no stray byte reaches the client's
// local service.
func TestRejectedHandshakeSignalsOnlyTheControlConnection(t *testing.T) {
	controlPort := freeTCPPort(t)
	engine, err := New(Config{ControlPort: controlPort, PortMin: freeTCPPort(t), PortMax: 65535, PublicHost: "tunnels.example.test"})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("start engine: %v", err)
	}
	defer engine.Stop()

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort))

	// No tunnel was ever provisioned, so this token is unknown.
	control, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("dial control port: %v", err)
	}
	defer control.Close()
	writeHandshake(t, control, "an-unknown-token", roleControl)
	_ = control.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, 1)
	if _, err := io.ReadFull(control, buffer); err != nil {
		t.Fatalf("expected a rejection byte on the control connection: %v", err)
	}
	if buffer[0] != signalAuthRejected {
		t.Fatalf("expected signalAuthRejected, got 0x%02x", buffer[0])
	}

	leg, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("dial control port: %v", err)
	}
	defer leg.Close()
	writeHandshake(t, leg, "an-unknown-token", roleTCPLeg)
	_ = leg.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(leg, buffer); err != io.EOF {
		t.Fatalf("expected a data leg to be closed without a signal, got byte 0x%02x err %v", buffer[0], err)
	}
}
