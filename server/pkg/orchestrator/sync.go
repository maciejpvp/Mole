package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// StartUsageSync flushes in-memory usage deltas to the control plane every
// interval. The control-plane response identifies tunnels to close immediately.
func StartUsageSync(ctx context.Context, engine *Engine, endpoint, token string, interval time.Duration) {
	if endpoint == "" || token == "" {
		log.Printf("[orchestrator] usage sync disabled: endpoint or token is missing")
		return
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	client := &http.Client{Timeout: 15 * time.Second}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncUsage(ctx, client, engine, endpoint, token)
			}
		}
	}()
}

func syncUsage(ctx context.Context, client *http.Client, engine *Engine, endpoint, token string) {
	updates := engine.CollectUsage(time.Now())
	if len(updates) == 0 {
		return
	}
	payload, err := json.Marshal(map[string]any{"updates": updates})
	if err != nil {
		log.Printf("[orchestrator] marshal usage sync: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/internal/v1/tunnels/usage", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[orchestrator] create usage sync request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		log.Printf("[orchestrator] usage sync: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("[orchestrator] usage sync failed: %s", response.Status)
		return
	}
	var result struct {
		StopTunnelIDs []string `json:"stop_tunnel_ids"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		log.Printf("[orchestrator] decode usage sync response: %v", err)
		return
	}
	engine.AcknowledgeUsage(updates)
	for _, tunnelID := range result.StopTunnelIDs {
		if err := engine.Deprovision(tunnelID); err != nil {
			log.Printf("[orchestrator] deprovision usage-limited tunnel %s: %v", tunnelID, err)
		}
	}
}
