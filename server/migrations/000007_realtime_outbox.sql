-- +goose Up
CREATE TABLE event_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    queue_ticket_id uuid,
    event_type text NOT NULL CHECK (char_length(event_type) BETWEEN 1 AND 100),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    FOREIGN KEY (queue_ticket_id, restaurant_id)
        REFERENCES queue_tickets (id, restaurant_id) ON DELETE RESTRICT
);

CREATE INDEX event_outbox_pending_idx
    ON event_outbox (created_at, id)
    WHERE published_at IS NULL;

-- +goose Down
DROP TABLE event_outbox;
