package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type readinessChecker interface {
	PingContext(context.Context) error
}

func healthHandler(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := writeSuccess(w, http.StatusOK, "Service is running.", map[string]string{
			"status": "ok",
		}); err != nil {
			logger.ErrorContext(r.Context(), "write liveness response", "error", err)
		}
	}
}

func readinessHandler(logger *slog.Logger, checker readinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		if err := checker.PingContext(ctx); err != nil {
			writeMappedError(
				w,
				r,
				logger,
				&APIError{
					Status:  http.StatusServiceUnavailable,
					Code:    "SERVICE_NOT_READY",
					Message: "The service is not ready.",
				},
			)
			return
		}

		if err := writeSuccess(w, http.StatusOK, "Service is ready.", map[string]string{
			"status": "ready",
		}); err != nil {
			logger.ErrorContext(r.Context(), "write readiness response", "error", err)
		}
	}
}
