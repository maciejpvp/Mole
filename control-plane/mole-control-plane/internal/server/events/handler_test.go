package events

import (
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
