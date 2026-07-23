package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"mole-control-plane/internal/tunnel"
)

type Event struct {
	Name string
	Data any
}

type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

func (b *Broker) Subscribe(userID string) chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 16)
	if _, exists := b.subscribers[userID]; !exists {
		b.subscribers[userID] = make(map[chan Event]struct{})
	}
	b.subscribers[userID][ch] = struct{}{}
	return ch
}

func (b *Broker) Unsubscribe(userID string, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if clients, exists := b.subscribers[userID]; exists {
		delete(clients, ch)
		close(ch)
		if len(clients) == 0 {
			delete(b.subscribers, userID)
		}
	}
}

func (b *Broker) Broadcast(userID string, event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	clients, exists := b.subscribers[userID]
	if !exists {
		return
	}

	for ch := range clients {
		select {
		case ch <- event:
		default:
			// Non-blocking send: skip slow consumers
		}
	}
}

func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "streaming unsupported"})
		return
	}

	account, err := s.authenticatedUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.broker.Subscribe(account.ID)
	defer s.broker.Unsubscribe(account.ID, ch)

	// Send initial snapshot
	profile, err := s.users.Profile(r.Context(), account.ID)
	if err == nil {
		s.writeSSE(w, flusher, "tunnel_update", profile)
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			s.writeSSE(w, flusher, event.Name, event.Data)
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) writeSSE(w http.ResponseWriter, flusher http.Flusher, eventName string, data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(jsonData))
	flusher.Flush()
}

func (s *Server) notifyUserUpdate(ctx context.Context, userID string) {
	if userID == "" || s.users == nil || s.broker == nil {
		return
	}
	profile, err := s.users.Profile(ctx, userID)
	if err != nil {
		return
	}
	s.broker.Broadcast(userID, Event{
		Name: "tunnel_update",
		Data: profile,
	})
}

func (s *Server) notifyUserUpdateForTunnel(ctx context.Context, tunnelID string) {
	if s.tunnels == nil {
		return
	}
	userID, err := s.tunnels.GetUserIDForTunnel(ctx, tunnelID)
	if err != nil || userID == "" {
		return
	}
	s.notifyUserUpdate(ctx, userID)
}

func (s *Server) notifyUserUpdateForUsage(ctx context.Context, updates []tunnel.UsageUpdate) {
	if s.tunnels == nil {
		return
	}
	tunnelIDs := make([]string, len(updates))
	for i, u := range updates {
		tunnelIDs[i] = u.TunnelID
	}
	userIDs, err := s.tunnels.GetUserIDsForTunnels(ctx, tunnelIDs)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.notifyUserUpdate(ctx, userID)
	}
}
