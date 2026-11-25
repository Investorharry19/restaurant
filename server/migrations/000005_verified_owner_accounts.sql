-- +goose Up
ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz;

UPDATE users
SET email_verified_at = created_at;

ALTER TABLE users
    ALTER COLUMN email_verified_at SET DEFAULT now();

CREATE TABLE owner_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(session_token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX owner_sessions_user_expiry_idx
    ON owner_sessions (user_id, expires_at);

CREATE TABLE owner_email_verification_tokens (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE
        CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at)
);

ALTER TABLE restaurant_applications
    ADD COLUMN owner_user_id uuid REFERENCES users (id) ON DELETE RESTRICT;

ALTER TABLE restaurant_applications
    DROP CONSTRAINT restaurant_applications_check1;

ALTER TABLE restaurant_applications
    ADD CONSTRAINT restaurant_applications_owner_account_check
    CHECK (
        (
            status = 'PENDING'
            AND (
                (owner_user_id IS NOT NULL AND owner_password_hash IS NULL)
                OR (owner_user_id IS NULL AND owner_password_hash IS NOT NULL)
            )
            AND restaurant_id IS NULL
            AND reviewed_by IS NULL
            AND reviewed_at IS NULL
        )
        OR
        (
            status = 'APPROVED'
            AND owner_password_hash IS NULL
            AND restaurant_id IS NOT NULL
            AND reviewed_by IS NOT NULL
            AND reviewed_at IS NOT NULL
        )
        OR
        (
            status = 'REJECTED'
            AND owner_password_hash IS NULL
            AND restaurant_id IS NULL
            AND reviewed_by IS NOT NULL
            AND reviewed_at IS NOT NULL
        )
    );

CREATE INDEX restaurant_applications_owner_user_idx
    ON restaurant_applications (owner_user_id)
    WHERE owner_user_id IS NOT NULL;

-- +goose Down
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM restaurant_applications
        WHERE status = 'PENDING'
          AND owner_user_id IS NOT NULL
          AND owner_password_hash IS NULL
    ) THEN
        RAISE EXCEPTION 'cannot roll back while verified-owner applications are pending';
    END IF;
END
$$;

ALTER TABLE restaurant_applications
    DROP CONSTRAINT restaurant_applications_owner_account_check;

ALTER TABLE restaurant_applications
    ADD CONSTRAINT restaurant_applications_check1
    CHECK (
        (status = 'PENDING' AND owner_password_hash IS NOT NULL
            AND restaurant_id IS NULL AND reviewed_by IS NULL AND reviewed_at IS NULL)
        OR
        (status = 'APPROVED' AND owner_password_hash IS NULL AND restaurant_id IS NOT NULL
            AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
        OR
        (status = 'REJECTED' AND owner_password_hash IS NULL AND restaurant_id IS NULL
            AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL)
    );

DROP INDEX restaurant_applications_owner_user_idx;
ALTER TABLE restaurant_applications DROP COLUMN owner_user_id;
DROP TABLE owner_email_verification_tokens;
DROP TABLE owner_sessions;
ALTER TABLE users DROP COLUMN email_verified_at;
