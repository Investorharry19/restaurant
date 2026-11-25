package realtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

type Publisher struct {
	db     *sql.DB
	logger *slog.Logger
	hub    *Hub
}

func NewPublisher(db *sql.DB, logger *slog.Logger, hub *Hub) *Publisher {
	return &Publisher{db: db, logger: logger, hub: hub}
}

func (publisher *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		count, err := publisher.publishBatch(ctx)
		if err != nil && ctx.Err() == nil {
			publisher.logger.ErrorContext(ctx, "publish realtime outbox batch", "error", err)
		}
		if err == nil && count > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (publisher *Publisher) publishBatch(ctx context.Context) (int, error) {
	tx, err := publisher.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin outbox batch: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id::text, restaurant_id::text, COALESCE(queue_ticket_id::text, ''),
		       event_type, payload, created_at
		FROM event_outbox
		WHERE published_at IS NULL
		ORDER BY created_at, id
		LIMIT 100
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		return 0, fmt.Errorf("load pending outbox events: %w", err)
	}

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var payload []byte
		if err := rows.Scan(
			&event.ID,
			&event.RestaurantID,
			&event.TicketID,
			&event.Type,
			&payload,
			&event.Timestamp,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan pending outbox event: %w", err)
		}
		if !json.Valid(payload) {
			rows.Close()
			return 0, fmt.Errorf("outbox event %s contains invalid JSON", event.ID)
		}
		event.Data = append(json.RawMessage(nil), payload...)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("read pending outbox events: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close pending outbox events: %w", err)
	}

	for _, event := range events {
		publisher.hub.Publish(event)
		if _, err := tx.ExecContext(ctx, `
			UPDATE event_outbox SET published_at = now()
			WHERE id = $1 AND published_at IS NULL
		`, event.ID); err != nil {
			return 0, fmt.Errorf("mark outbox event %s published: %w", event.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit published outbox batch: %w", err)
	}
	return len(events), nil
}
