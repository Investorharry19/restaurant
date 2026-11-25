-- +goose Up
CREATE TABLE restaurant_tables (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    capacity smallint NOT NULL CHECK (capacity BETWEEN 1 AND 30),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, restaurant_id)
);

CREATE UNIQUE INDEX restaurant_tables_name_unique
    ON restaurant_tables (restaurant_id, lower(name));

ALTER TABLE queue_tickets
    ADD CONSTRAINT queue_tickets_id_restaurant_unique UNIQUE (id, restaurant_id);

CREATE TABLE table_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    table_id uuid NOT NULL,
    queue_ticket_id uuid NOT NULL,
    assigned_by_user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz,
    FOREIGN KEY (table_id, restaurant_id)
        REFERENCES restaurant_tables (id, restaurant_id) ON DELETE RESTRICT,
    FOREIGN KEY (queue_ticket_id, restaurant_id)
        REFERENCES queue_tickets (id, restaurant_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX table_assignments_active_table_unique
    ON table_assignments (table_id)
    WHERE released_at IS NULL;
CREATE UNIQUE INDEX table_assignments_active_ticket_unique
    ON table_assignments (queue_ticket_id)
    WHERE released_at IS NULL;
CREATE INDEX table_assignments_restaurant_active_idx
    ON table_assignments (restaurant_id, assigned_at DESC)
    WHERE released_at IS NULL;

CREATE TABLE restaurant_staff_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    email text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 320),
    role text NOT NULL CHECK (role IN ('MANAGER', 'STAFF')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    invited_by_user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    CHECK (expires_at > created_at)
);

CREATE INDEX restaurant_staff_invitations_pending_idx
    ON restaurant_staff_invitations (restaurant_id, lower(email), expires_at)
    WHERE accepted_at IS NULL;

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    customer_session_id uuid REFERENCES customer_sessions (id) ON DELETE SET NULL,
    queue_ticket_id uuid REFERENCES queue_tickets (id) ON DELETE SET NULL,
    order_id uuid REFERENCES orders (id) ON DELETE SET NULL,
    type text NOT NULL CHECK (char_length(type) BETWEEN 1 AND 80),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    status text NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'SENT', 'FAILED', 'CANCELLED')),
    idempotency_key text NOT NULL UNIQUE CHECK (char_length(idempotency_key) BETWEEN 1 AND 200),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at timestamptz,
    locked_at timestamptz,
    locked_by text,
    last_error_code text CHECK (last_error_code IS NULL OR char_length(last_error_code) <= 100),
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_restaurant_created_idx
    ON notifications (restaurant_id, created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE restaurant_staff_invitations;
DROP TABLE table_assignments;
ALTER TABLE queue_tickets
    DROP CONSTRAINT queue_tickets_id_restaurant_unique;
DROP TABLE restaurant_tables;
