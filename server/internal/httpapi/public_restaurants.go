package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
)

type publicRestaurant struct {
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	LogoURL           *string  `json:"logoUrl"`
	IsActive          bool     `json:"isActive"`
	EnabledQueueTypes []string `json:"enabledQueueTypes"`
}

type publicRestaurantReader interface {
	FindBySlug(context.Context, string) (publicRestaurant, error)
	FindLogoBySlug(context.Context, string) ([]byte, string, error)
}

func (reader postgresPublicRestaurantReader) FindLogoBySlug(
	ctx context.Context,
	slug string,
) ([]byte, string, error) {
	var data []byte
	var contentType string
	err := reader.db.QueryRowContext(ctx, `
		SELECT logo_data, logo_content_type
		FROM restaurants
		WHERE slug = $1 AND logo_data IS NOT NULL
	`, slug).Scan(&data, &contentType)
	if err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}

type postgresPublicRestaurantReader struct {
	db *sql.DB
}

func (reader postgresPublicRestaurantReader) FindBySlug(
	ctx context.Context,
	slug string,
) (publicRestaurant, error) {
	const query = `
		SELECT name, description, is_active, table_queue_enabled, pickup_queue_enabled,
		       logo_data IS NOT NULL
		FROM restaurants
		WHERE slug = $1
	`

	var restaurant publicRestaurant
	var description sql.NullString
	var tableQueueEnabled bool
	var pickupQueueEnabled bool
	var hasLogo bool
	if err := reader.db.QueryRowContext(ctx, query, slug).Scan(
		&restaurant.Name,
		&description,
		&restaurant.IsActive,
		&tableQueueEnabled,
		&pickupQueueEnabled,
		&hasLogo,
	); err != nil {
		return publicRestaurant{}, err
	}

	if description.Valid {
		restaurant.Description = &description.String
	}
	if hasLogo {
		logoURL := "/api/public/restaurants/" + url.PathEscape(slug) + "/logo"
		restaurant.LogoURL = &logoURL
	}
	restaurant.EnabledQueueTypes = enabledQueueTypes(tableQueueEnabled, pickupQueueEnabled)
	return restaurant, nil
}

func enabledQueueTypes(tableEnabled, pickupEnabled bool) []string {
	types := make([]string, 0, 2)
	if tableEnabled {
		types = append(types, "TABLE")
	}
	if pickupEnabled {
		types = append(types, "PICKUP")
	}
	return types
}

func registerPublicRestaurantRoutes(
	mux *http.ServeMux,
	reader publicRestaurantReader,
	logger *slog.Logger,
) {
	mux.HandleFunc(
		"GET /api/public/restaurants/{slug}",
		publicRestaurantHandler(reader, logger),
	)
	mux.HandleFunc(
		"GET /api/public/restaurants/{slug}/logo",
		publicRestaurantLogoHandler(reader, logger),
	)
}

func publicRestaurantLogoHandler(reader publicRestaurantReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, contentType, err := reader.FindLogoBySlug(r.Context(), r.PathValue("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, logger, &APIError{
				Status:  http.StatusNotFound,
				Code:    "RESTAURANT_LOGO_NOT_FOUND",
				Message: "Restaurant logo not found.",
			})
			return
		}
		if err != nil {
			writeMappedError(w, r, logger, fmt.Errorf("load public restaurant logo: %w", err))
			return
		}
		writeRestaurantLogo(w, r, logger, data, contentType, "public, max-age=3600")
	}
}

func publicRestaurantHandler(
	reader publicRestaurantReader,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		restaurant, err := reader.FindBySlug(r.Context(), r.PathValue("slug"))
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, logger, &APIError{
				Status:  http.StatusNotFound,
				Code:    "RESTAURANT_NOT_FOUND",
				Message: "Restaurant not found.",
			})
			return
		}
		if err != nil {
			writeMappedError(w, r, logger, err)
			return
		}

		if err := writeSuccess(w, http.StatusOK, "Restaurant loaded.", restaurant); err != nil {
			loggerFromContext(r.Context(), logger).ErrorContext(
				r.Context(),
				"write public restaurant response",
				"error", err,
			)
		}
	}
}
