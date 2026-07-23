package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSEBrokerSubscribeAndBroadcast(t *testing.T) {
	broker := NewBroker()
	userID := "user-123"

	ch := broker.Subscribe(userID)

	testEvent := Event{
		Name: "tunnel_update",
		Data: map[string]string{"status": "active"},
	}

	broker.Broadcast(userID, testEvent)

	select {
	case evt := <-ch:
		if evt.Name != "tunnel_update" {
			t.Fatalf("expected event name tunnel_update, got %s", evt.Name)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}

	broker.Unsubscribe(userID, ch)

	broker.Broadcast(userID, testEvent)
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for closed channel check")
	}
}

func TestSSEEventsRouteRequiresAuthentication(t *testing.T) {
	s := &Server{broker: NewBroker()}
	routes := s.RegisterRoutes()

	routesToTest := []string{"/api/v1/tunnels/events", "/api/v1/events"}
	for _, route := range routesToTest {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, route, nil)
		routes.ServeHTTP(recorder, req)

		resp := recorder.Result()
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("route %s: expected status unauthorized (401), got %v", route, resp.Status)
		}
	}
}
