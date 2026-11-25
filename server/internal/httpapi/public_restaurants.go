package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
)

type publicRestaurant struct {
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	IsActive          bool     `json:"isActive"`
	EnabledQueueTypes []string `json:"enabledQueueTypes"`
}

type publicRestaurantReader interface {
	FindBySlug(context.Context, string) (publicRestaurant, error)
}

type postgresPublicRestaurantReader struct {
	db *sql.DB
}

func (reader postgresPublicRestaurantReader) FindBySlug(
	ctx context.Context,
	slug string,
) (publicRestaurant, error) {
	const query = `
		SELECT name, description, is_active, table_queue_enabled, pickup_queue_enabled
		FROM restaurants
		WHERE slug = $1
	`

	var restaurant publicRestaurant
	var description sql.NullString
	var tableQueueEnabled bool
	var pickupQueueEnabled bool
	if err := reader.db.QueryRowContext(ctx, query, slug).Scan(
		&restaurant.Name,
		&description,
		&restaurant.IsActive,
		&tableQueueEnabled,
		&pickupQueueEnabled,
	); err != nil {
		return publicRestaurant{}, err
	}

	if description.Valid {
		restaurant.Description = &description.String
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
