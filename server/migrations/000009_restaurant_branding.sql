-- +goose Up
ALTER TABLE restaurants
    ADD COLUMN logo_data bytea,
    ADD COLUMN logo_content_type text;

ALTER TABLE restaurant_applications
    ADD COLUMN logo_data bytea,
    ADD COLUMN logo_content_type text;

ALTER TABLE restaurants
    ADD CONSTRAINT restaurants_logo_pair_check
        CHECK ((logo_data IS NULL) = (logo_content_type IS NULL)),
    ADD CONSTRAINT restaurants_logo_content_type_check
        CHECK (logo_content_type IS NULL OR logo_content_type IN ('image/png', 'image/jpeg')),
    ADD CONSTRAINT restaurants_logo_size_check
        CHECK (logo_data IS NULL OR octet_length(logo_data) <= 524288);

ALTER TABLE restaurant_applications
    ADD CONSTRAINT restaurant_applications_logo_pair_check
        CHECK ((logo_data IS NULL) = (logo_content_type IS NULL)),
    ADD CONSTRAINT restaurant_applications_logo_content_type_check
        CHECK (logo_content_type IS NULL OR logo_content_type IN ('image/png', 'image/jpeg')),
    ADD CONSTRAINT restaurant_applications_logo_size_check
        CHECK (logo_data IS NULL OR octet_length(logo_data) <= 524288);

-- +goose Down
ALTER TABLE restaurant_applications
    DROP CONSTRAINT restaurant_applications_logo_size_check,
    DROP CONSTRAINT restaurant_applications_logo_content_type_check,
    DROP CONSTRAINT restaurant_applications_logo_pair_check,
    DROP COLUMN logo_content_type,
    DROP COLUMN logo_data;

ALTER TABLE restaurants
    DROP CONSTRAINT restaurants_logo_size_check,
    DROP CONSTRAINT restaurants_logo_content_type_check,
    DROP CONSTRAINT restaurants_logo_pair_check,
    DROP COLUMN logo_content_type,
    DROP COLUMN logo_data;
