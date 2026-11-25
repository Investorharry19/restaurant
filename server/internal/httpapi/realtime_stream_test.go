package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"restaurant/server/internal/realtime"
)

type streamTestWriter struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
}

func (writer *streamTestWriter) SetWriteDeadline(time.Time) error {
	return nil
}

func (writer *streamTestWriter) Flush() {
	writer.ResponseRecorder.Flush()
	select {
	case writer.flushed <- struct{}{}:
	default:
	}
}

func TestStreamEventsFlushesEventsAndStopsOnDisconnect(t *testing.T) {
	hub := realtime.NewHub()
	subscription := hub.SubscribeRestaurant("restaurant-1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest("GET", "/events", nil).WithContext(ctx)
	recorder := &streamTestWriter{
		ResponseRecorder: httptest.NewRecorder(),
		flushed:          make(chan struct{}, 2),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		streamEvents(
			recorder,
			request,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
			subscription,
			nil,
		)
	}()

	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("event stream did not flush its connection comment")
	}
	hub.Publish(realtime.Event{
		ID:           "event-1",
		Type:         "queue.ticket.updated",
		Timestamp:    time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		Data:         json.RawMessage(`{"status":"CALLED"}`),
		RestaurantID: "restaurant-1",
	})
	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("event stream did not flush its event")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream did not stop after disconnect")
	}

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, ": connected\n\n") ||
		!strings.Contains(body, "id: event-1\n") ||
		!strings.Contains(body, `"type":"queue.ticket.updated"`) ||
		!strings.Contains(body, `"status":"CALLED"`) {
		t.Fatalf("event stream body is incomplete: %q", body)
	}
}
