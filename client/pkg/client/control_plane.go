package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// TunnelConfig is fetched from the control plane after the client proves
// possession of a tunnel token.
type TunnelConfig struct {
	Protocol        string `json:"proto"`
	InternalAddress string `json:"internal_address"`
	ServerAddress   string `json:"server_address"`
}

func FetchTunnelConfig(ctx context.Context, controlPlaneURL, token string) (TunnelConfig, error) {
	if strings.TrimSpace(controlPlaneURL) == "" || strings.TrimSpace(token) == "" {
		return TunnelConfig{}, fmt.Errorf("control plane URL and tunnel token are required")
	}
	controlPlaneURL = strings.TrimSpace(controlPlaneURL)
	if !strings.Contains(controlPlaneURL, "://") {
		controlPlaneURL = "http://" + controlPlaneURL
	}
	body, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return TunnelConfig{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(controlPlaneURL, "/")+"/api/v1/tunnels/connect", bytes.NewReader(body))
	if err != nil {
		return TunnelConfig{}, fmt.Errorf("create tunnel configuration request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return TunnelConfig{}, fmt.Errorf("fetch tunnel configuration: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return TunnelConfig{}, fmt.Errorf("fetch tunnel configuration: %s", response.Status)
	}
	var config TunnelConfig
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		return TunnelConfig{}, fmt.Errorf("decode tunnel configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return TunnelConfig{}, err
	}
	return config, nil
}

func (c TunnelConfig) Validate() error {
	if c.Protocol != "tcp" && c.Protocol != "udp" {
		return fmt.Errorf("invalid tunnel protocol")
	}
	if _, _, err := net.SplitHostPort(c.InternalAddress); err != nil {
		return fmt.Errorf("invalid internal address")
	}
	if _, _, err := net.SplitHostPort(c.ServerAddress); err != nil {
		return fmt.Errorf("invalid server address")
	}
	return nil
}
