-- +goose Up
ALTER TABLE queue_tickets
    ADD CONSTRAINT queue_tickets_id_session_restaurant_unique
    UNIQUE (id, customer_session_id, restaurant_id);

CREATE TABLE push_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL,
    customer_session_id uuid NOT NULL,
    queue_ticket_id uuid NOT NULL,
    endpoint text NOT NULL CHECK (char_length(endpoint) BETWEEN 1 AND 2048),
    endpoint_hash bytea NOT NULL CHECK (octet_length(endpoint_hash) = 32),
    p256dh text NOT NULL CHECK (char_length(p256dh) BETWEEN 1 AND 128),
    auth text NOT NULL CHECK (char_length(auth) BETWEEN 1 AND 128),
    encrypted_ticket_token bytea NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    FOREIGN KEY (queue_ticket_id, customer_session_id, restaurant_id)
        REFERENCES queue_tickets (id, customer_session_id, restaurant_id) ON DELETE RESTRICT,
    UNIQUE (endpoint_hash, queue_ticket_id)
);

CREATE INDEX push_subscriptions_ticket_active_idx
    ON push_subscriptions (queue_ticket_id, created_at)
    WHERE is_active;

ALTER TABLE notifications
    ADD COLUMN ticket_token_ciphertext bytea;

CREATE TABLE notification_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL REFERENCES notifications (id) ON DELETE RESTRICT,
    push_subscription_id uuid REFERENCES push_subscriptions (id) ON DELETE SET NULL,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    outcome text NOT NULL CHECK (outcome IN ('SENT', 'RETRY', 'INVALID', 'FAILED')),
    response_status smallint CHECK (response_status IS NULL OR response_status BETWEEN 100 AND 599),
    error_code text CHECK (error_code IS NULL OR char_length(error_code) <= 100),
    attempted_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notification_attempts_notification_idx
    ON notification_attempts (notification_id, attempt_number);

CREATE INDEX notifications_due_idx
    ON notifications (next_attempt_at, created_at)
    WHERE status IN ('PENDING', 'PROCESSING');

-- +goose Down
DROP INDEX notifications_due_idx;
DROP TABLE notification_attempts;
ALTER TABLE notifications DROP COLUMN ticket_token_ciphertext;
DROP INDEX push_subscriptions_ticket_active_idx;
DROP TABLE push_subscriptions;
ALTER TABLE queue_tickets
    DROP CONSTRAINT queue_tickets_id_session_restaurant_unique;
