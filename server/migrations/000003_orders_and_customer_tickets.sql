-- +goose Up
CREATE TABLE restaurant_order_counters (
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    business_date date NOT NULL,
    next_order_number bigint NOT NULL DEFAULT 1 CHECK (next_order_number > 0),
    PRIMARY KEY (restaurant_id, business_date)
);

CREATE TABLE orders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    business_date date NOT NULL,
    order_number bigint NOT NULL CHECK (order_number > 0),
    pickup_code_hash bytea NOT NULL CHECK (octet_length(pickup_code_hash) = 32),
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'ACCEPTED', 'PREPARING', 'READY', 'PICKED_UP', 'CANCELLED')),
    notes text CHECK (notes IS NULL OR char_length(notes) <= 2000),
    created_by_user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    preparing_at timestamptz,
    ready_at timestamptz,
    picked_up_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (restaurant_id, business_date, order_number),
    UNIQUE (restaurant_id, pickup_code_hash),
    UNIQUE (id, restaurant_id)
);

CREATE INDEX orders_restaurant_status_created_idx
    ON orders (restaurant_id, status, created_at);

CREATE TABLE customer_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    display_name text NOT NULL CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 120),
    phone text CHECK (phone IS NULL OR char_length(phone) <= 50),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (id, restaurant_id),
    CHECK (expires_at > created_at)
);

CREATE INDEX customer_sessions_restaurant_expiry_idx
    ON customer_sessions (restaurant_id, expires_at);

CREATE TABLE queues (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    type text NOT NULL CHECK (type IN ('TABLE', 'PICKUP')),
    business_date date NOT NULL,
    is_open boolean NOT NULL DEFAULT true,
    next_ticket_number bigint NOT NULL DEFAULT 1 CHECK (next_ticket_number > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (restaurant_id, type, business_date),
    UNIQUE (id, restaurant_id)
);

CREATE TABLE queue_tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    queue_id uuid NOT NULL,
    customer_session_id uuid NOT NULL,
    order_id uuid,
    ticket_number bigint NOT NULL CHECK (ticket_number > 0),
    public_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(public_token_hash) = 32),
    type text NOT NULL CHECK (type IN ('TABLE', 'PICKUP')),
    status text NOT NULL DEFAULT 'WAITING'
        CHECK (status IN (
            'WAITING', 'CALLED', 'ARRIVED', 'SEATED', 'NO_SHOW',
            'PREPARING', 'READY', 'PICKED_UP', 'CANCELLED', 'EXPIRED'
        )),
    party_size integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    called_at timestamptz,
    arrived_at timestamptz,
    completed_at timestamptz,
    expires_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (queue_id, ticket_number),
    UNIQUE (order_id),
    FOREIGN KEY (queue_id, restaurant_id) REFERENCES queues (id, restaurant_id) ON DELETE RESTRICT,
    FOREIGN KEY (customer_session_id, restaurant_id)
        REFERENCES customer_sessions (id, restaurant_id) ON DELETE RESTRICT,
    FOREIGN KEY (order_id, restaurant_id) REFERENCES orders (id, restaurant_id) ON DELETE RESTRICT,
    CHECK (
        (type = 'TABLE' AND order_id IS NULL AND party_size IS NOT NULL AND party_size BETWEEN 1 AND 20
            AND status IN ('WAITING', 'CALLED', 'ARRIVED', 'SEATED', 'NO_SHOW', 'CANCELLED', 'EXPIRED'))
        OR
        (type = 'PICKUP' AND order_id IS NOT NULL AND party_size IS NULL
            AND status IN ('WAITING', 'PREPARING', 'READY', 'PICKED_UP', 'CANCELLED', 'EXPIRED'))
    )
);

CREATE INDEX queue_tickets_position_idx
    ON queue_tickets (queue_id, status, created_at);
CREATE INDEX queue_tickets_restaurant_status_idx
    ON queue_tickets (restaurant_id, status, created_at);

-- +goose Down
DROP TABLE queue_tickets;
DROP TABLE queues;
DROP TABLE customer_sessions;
DROP TABLE orders;
DROP TABLE restaurant_order_counters;
