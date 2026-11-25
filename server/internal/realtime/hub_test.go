package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHubScopesRestaurantAndTicketEvents(t *testing.T) {
	hub := NewHub()
	restaurant := hub.SubscribeRestaurant("restaurant-1")
	ticket := hub.SubscribeTicket("restaurant-1", "ticket-1")
	otherTicket := hub.SubscribeTicket("restaurant-1", "ticket-2")
	otherRestaurant := hub.SubscribeRestaurant("restaurant-2")
	defer restaurant.Close()
	defer ticket.Close()
	defer otherTicket.Close()
	defer otherRestaurant.Close()

	event := Event{
		ID:           "event-1",
		Type:         "queue.ticket.updated",
		Timestamp:    time.Now(),
		Data:         json.RawMessage(`{"status":"CALLED"}`),
		RestaurantID: "restaurant-1",
		TicketID:     "ticket-1",
	}
	hub.Publish(event)

	if got := <-restaurant.Events(); got.ID != event.ID {
		t.Fatalf("restaurant event ID = %q, want %q", got.ID, event.ID)
	}
	if got := <-ticket.Events(); got.ID != event.ID {
		t.Fatalf("ticket event ID = %q, want %q", got.ID, event.ID)
	}
	assertNoEvent(t, otherTicket.Events())
	assertNoEvent(t, otherRestaurant.Events())
}

func TestHubClosesSlowSubscribers(t *testing.T) {
	hub := NewHub()
	subscription := hub.SubscribeRestaurant("restaurant-1")

	for i := 0; i < cap(subscription.events); i++ {
		hub.Publish(Event{RestaurantID: "restaurant-1"})
	}
	hub.Publish(Event{RestaurantID: "restaurant-1"})

	for range subscription.Events() {
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if _, ok := hub.subscribers[subscription.id]; ok {
		t.Fatal("slow subscriber remained registered")
	}
}

func assertNoEvent(t *testing.T, events <-chan Event) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("received unexpected event %#v", event)
	default:
	}
}
