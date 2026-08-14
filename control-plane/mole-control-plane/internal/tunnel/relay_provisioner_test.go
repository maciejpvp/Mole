package tunnel

import (
	"context"
	"net"
	"testing"

	"mole-control-plane/internal/relay"
)

func TestRelayProvisionerProvisionsAndDeprovisions(t *testing.T) {
	port := freeRelayPort(t)
	engine, err := relay.New(relay.Config{ControlPort: 9000, PortMin: port, PortMax: port, PublicHost: "relay.example.test"})
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}
	provisioner := NewRelayProvisioner(engine)
	response, err := provisioner.Provision(context.Background(), ProvisionRequest{TunnelID: "tunnel-1", UserID: "user-1", Protocol: "tcp", Token: "token"})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if response.OutboundPort != port || response.PublicHost != "relay.example.test" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if err := provisioner.Deprovision(context.Background(), "tunnel-1"); err != nil {
		t.Fatalf("deprovision: %v", err)
	}
}

func freeRelayPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
