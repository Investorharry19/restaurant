package httpapi

import (
	"context"
	"database/sql"
	"fmt"
)

func enqueueReadyPushNotification(
	ctx context.Context,
	tx *sql.Tx,
	restaurantID string,
	ticketID string,
	notificationType string,
	title string,
	body string,
) error {
	idempotencyKey := ticketID + ":" + notificationType
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO notifications (
			restaurant_id, customer_session_id, queue_ticket_id, order_id,
			type, title, body, status, idempotency_key, next_attempt_at,
			ticket_token_ciphertext
		)
		SELECT t.restaurant_id, t.customer_session_id, t.id, t.order_id,
		       $3, $4, $5, 'PENDING', $6, now(), subscription.encrypted_ticket_token
		FROM queue_tickets t
		JOIN LATERAL (
		    SELECT encrypted_ticket_token
		    FROM push_subscriptions
		    WHERE queue_ticket_id = t.id
		      AND restaurant_id = t.restaurant_id
		      AND is_active
		    ORDER BY created_at, id
		    LIMIT 1
		) subscription ON true
		WHERE t.id = $1 AND t.restaurant_id = $2
		ON CONFLICT (idempotency_key) DO NOTHING
	`, ticketID, restaurantID, notificationType, title, body, idempotencyKey); err != nil {
		return fmt.Errorf("enqueue %s push notification: %w", notificationType, err)
	}
	return nil
}
