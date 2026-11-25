-- +goose Up
ALTER TABLE restaurants
    ADD COLUMN table_queue_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN pickup_queue_enabled boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE restaurants
    DROP COLUMN pickup_queue_enabled,
    DROP COLUMN table_queue_enabled;
