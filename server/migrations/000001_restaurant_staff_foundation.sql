-- +goose Up
CREATE TABLE restaurants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    slug text NOT NULL UNIQUE
        CHECK (slug = lower(slug) AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description text CHECK (description IS NULL OR char_length(description) <= 2000),
    address text CHECK (address IS NULL OR char_length(address) <= 500),
    phone text CHECK (phone IS NULL OR char_length(phone) <= 50),
    timezone text NOT NULL DEFAULT 'UTC',
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    email text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 320),
    password_hash text NOT NULL CHECK (char_length(password_hash) BETWEEN 1 AND 255),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_lower_unique ON users (lower(email));

CREATE TABLE restaurant_memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role text NOT NULL CHECK (role IN ('OWNER', 'MANAGER', 'STAFF')),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (restaurant_id, user_id)
);

CREATE TABLE staff_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(session_token_hash) = 32),
    active_restaurant_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    ip_address inet,
    user_agent text CHECK (user_agent IS NULL OR char_length(user_agent) <= 1000),
    FOREIGN KEY (active_restaurant_id, user_id)
        REFERENCES restaurant_memberships (restaurant_id, user_id)
        ON DELETE RESTRICT,
    CHECK (expires_at > created_at)
);

CREATE INDEX staff_sessions_user_expiry_idx ON staff_sessions (user_id, expires_at);
CREATE INDEX staff_sessions_expiry_idx ON staff_sessions (expires_at);

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE RESTRICT,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 100),
    resource_type text NOT NULL CHECK (char_length(resource_type) BETWEEN 1 AND 100),
    resource_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    request_id text CHECK (request_id IS NULL OR char_length(request_id) <= 128),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_restaurant_created_idx
    ON audit_logs (restaurant_id, created_at DESC);
CREATE INDEX audit_logs_actor_created_idx
    ON audit_logs (actor_user_id, created_at DESC);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE staff_sessions;
DROP TABLE restaurant_memberships;
DROP TABLE users;
DROP TABLE restaurants;
