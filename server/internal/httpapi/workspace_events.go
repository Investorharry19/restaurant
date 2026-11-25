package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

func recordRestaurantAudit(
	ctx context.Context,
	tx *sql.Tx,
	identity staffIdentity,
	action string,
	resourceType string,
	resourceID string,
	metadata any,
) error {
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			restaurant_id, actor_user_id, action, resource_type, resource_id, metadata
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, $6)
	`, identity.restaurantID, identity.userID, action, resourceType, resourceID, encodedMetadata); err != nil {
		return fmt.Errorf("write restaurant audit log: %w", err)
	}
	return nil
}

func enqueueRestaurantEvent(
	ctx context.Context,
	tx *sql.Tx,
	restaurantID string,
	ticketID string,
	eventType string,
	data any,
) error {
	encodedData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode outbox event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_outbox (restaurant_id, queue_ticket_id, event_type, payload)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4)
	`, restaurantID, ticketID, eventType, encodedData); err != nil {
		return fmt.Errorf("write realtime outbox event: %w", err)
	}
	return nil
}
