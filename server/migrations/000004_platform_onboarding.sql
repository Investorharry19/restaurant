-- +goose Up
CREATE TABLE platform_admins (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
    email text NOT NULL CHECK (char_length(email) BETWEEN 3 AND 320),
    password_hash text NOT NULL CHECK (char_length(password_hash) BETWEEN 1 AND 255),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX platform_admins_email_lower_unique
    ON platform_admins (lower(email));

CREATE TABLE platform_admin_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id uuid NOT NULL REFERENCES platform_admins (id) ON DELETE CASCADE,
    session_token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(session_token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX platform_admin_sessions_admin_expiry_idx
    ON platform_admin_sessions (admin_id, expires_at);

CREATE TABLE restaurant_applications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_name text NOT NULL
        CHECK (char_length(btrim(business_name)) BETWEEN 1 AND 120),
    business_type text NOT NULL
        CHECK (business_type IN ('RESTAURANT', 'BAKERY', 'EATERY')),
    slug text NOT NULL
        CHECK (slug = lower(slug) AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    address text CHECK (address IS NULL OR char_length(address) <= 500),
    phone text CHECK (phone IS NULL OR char_length(phone) <= 50),
    owner_name text NOT NULL CHECK (char_length(btrim(owner_name)) BETWEEN 1 AND 120),
    owner_email text NOT NULL CHECK (char_length(owner_email) BETWEEN 3 AND 320),
    owner_password_hash text CHECK (
        owner_password_hash IS NULL OR char_length(owner_password_hash) BETWEEN 1 AND 255
    ),
    table_queue_enabled boolean NOT NULL DEFAULT false,
    pickup_queue_enabled boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    status_token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(status_token_hash) = 32),
    restaurant_id uuid UNIQUE REFERENCES restaurants (id) ON DELETE RESTRICT,
    reviewed_by uuid REFERENCES platform_admins (id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    review_note text CHECK (review_note IS NULL OR char_length(review_note) <= 1000),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (table_queue_enabled OR pickup_queue_enabled),
    CHECK (
        (status = 'PENDING' AND owner_password_hash IS NOT NULL AND restaurant_id IS NULL
            AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR
        (status = 'APPROVED' AND owner_password_hash IS NULL AND restaurant_id IS NOT NULL
            AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
        OR
        (status = 'REJECTED' AND owner_password_hash IS NULL AND restaurant_id IS NULL
            AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX restaurant_applications_pending_slug_unique
    ON restaurant_applications (slug)
    WHERE status = 'PENDING';
CREATE UNIQUE INDEX restaurant_applications_pending_email_unique
    ON restaurant_applications (lower(owner_email))
    WHERE status = 'PENDING';
CREATE INDEX restaurant_applications_status_created_idx
    ON restaurant_applications (status, created_at DESC, id DESC);

CREATE TABLE platform_audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id uuid NOT NULL REFERENCES platform_admins (id) ON DELETE RESTRICT,
    action text NOT NULL CHECK (char_length(action) BETWEEN 1 AND 100),
    application_id uuid REFERENCES restaurant_applications (id) ON DELETE RESTRICT,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),
    request_id text CHECK (request_id IS NULL OR char_length(request_id) <= 128),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX platform_audit_logs_admin_created_idx
    ON platform_audit_logs (admin_id, created_at DESC);
CREATE INDEX platform_audit_logs_application_created_idx
    ON platform_audit_logs (application_id, created_at DESC);

-- +goose Down
DROP TABLE platform_audit_logs;
DROP TABLE restaurant_applications;
DROP TABLE platform_admin_sessions;
DROP TABLE platform_admins;
