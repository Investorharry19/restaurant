package httpapi

import (
	"database/sql"
	"log/slog"
	"net/http"
)

func NewHandler(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(logger))
	mux.HandleFunc("GET /readyz", readinessHandler(logger, db))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeMappedError(
			w,
			r,
			logger,
			&APIError{
				Status:  http.StatusNotFound,
				Code:    "NOT_FOUND",
				Message: "API route not found.",
			},
		)
	})

	return middleware(mux, logger, allowedOrigins, production)
}
