package realtime

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
	RestaurantID string          `json:"-"`
	TicketID     string          `json:"-"`
}

type subscription struct {
	restaurantID string
	ticketID     string
	events       chan Event
}

type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]subscription
}

type Subscription struct {
	hub    *Hub
	id     uint64
	events <-chan Event
	once   sync.Once
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[uint64]subscription)}
}

func (hub *Hub) SubscribeRestaurant(restaurantID string) *Subscription {
	return hub.subscribe(subscription{restaurantID: restaurantID})
}

func (hub *Hub) SubscribeTicket(restaurantID, ticketID string) *Subscription {
	return hub.subscribe(subscription{restaurantID: restaurantID, ticketID: ticketID})
}

func (hub *Hub) subscribe(subscriber subscription) *Subscription {
	subscriber.events = make(chan Event, 32)
	hub.mu.Lock()
	hub.nextID++
	id := hub.nextID
	hub.subscribers[id] = subscriber
	hub.mu.Unlock()
	return &Subscription{hub: hub, id: id, events: subscriber.events}
}

func (subscription *Subscription) Events() <-chan Event {
	return subscription.events
}

func (subscription *Subscription) Close() {
	subscription.once.Do(func() {
		subscription.hub.mu.Lock()
		if current, ok := subscription.hub.subscribers[subscription.id]; ok {
			delete(subscription.hub.subscribers, subscription.id)
			close(current.events)
		}
		subscription.hub.mu.Unlock()
	})
}

func (hub *Hub) Publish(event Event) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	for id, subscriber := range hub.subscribers {
		if subscriber.restaurantID != event.RestaurantID {
			continue
		}
		if subscriber.ticketID != "" && subscriber.ticketID != event.TicketID {
			continue
		}
		select {
		case subscriber.events <- event:
		default:
			delete(hub.subscribers, id)
			close(subscriber.events)
		}
	}
}
