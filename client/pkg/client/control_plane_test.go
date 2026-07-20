package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchTunnelConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tunnels/connect" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(body) != `{"token":"tunnel-token"}` {
			t.Fatalf("unexpected request body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"proto":"tcp","internal_address":"127.0.0.1:25565","server_address":"relay.example.test:9000"}`))
	}))
	defer server.Close()

	config, err := FetchTunnelConfig(context.Background(), server.URL, "tunnel-token")
	if err != nil {
		t.Fatalf("fetch tunnel configuration: %v", err)
	}
	if config.Protocol != "tcp" || config.InternalAddress != "127.0.0.1:25565" || config.ServerAddress != "relay.example.test:9000" {
		t.Fatalf("unexpected tunnel configuration: %+v", config)
	}
}

func TestTunnelConfigValidateRejectsInvalidConfig(t *testing.T) {
	config := TunnelConfig{Protocol: "icmp", InternalAddress: "127.0.0.1:25565", ServerAddress: "relay.example.test:9000"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected invalid protocol error")
	}
}
