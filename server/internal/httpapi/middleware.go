package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type contextKey int

const loggerContextKey contextKey = iota

func middleware(
	handler http.Handler,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
) http.Handler {
	handler = recoverPanics(logger, handler)
	handler = cors(allowedOrigins, logger, handler)
	handler = accessLog(logger, handler)
	handler = requestID(logger, handler)
	handler = securityHeaders(production, handler)
	return handler
}

func requestID(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idBytes := make([]byte, 16)
		if _, err := rand.Read(idBytes); err != nil {
			logger.ErrorContext(r.Context(), "generate request ID", "error", err)
			if writeErr := writeError(
				w,
				http.StatusInternalServerError,
				"INTERNAL_ERROR",
				"An unexpected error occurred.",
			); writeErr != nil {
				logger.ErrorContext(r.Context(), "write request ID error response", "error", writeErr)
			}
			return
		}

		id := hex.EncodeToString(idBytes)
		w.Header().Set("X-Request-ID", id)
		requestLogger := logger.With("request_id", id)
		ctx := context.WithValue(r.Context(), loggerContextKey, requestLogger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey).(*slog.Logger); ok {
		return logger
	}
	return fallback
}

func accessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		loggerFromContext(r.Context(), logger).InfoContext(
			r.Context(),
			"HTTP request",
			"method", r.Method,
			"route", r.Pattern,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func (r *responseRecorder) Flush() {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				loggerFromContext(r.Context(), logger).ErrorContext(
					r.Context(),
					"HTTP handler panic",
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				if recorder, ok := w.(*responseRecorder); ok && recorder.status != 0 {
					return
				}
				if err := writeError(
					w,
					http.StatusInternalServerError,
					"INTERNAL_ERROR",
					"An unexpected error occurred.",
				); err != nil {
					loggerFromContext(r.Context(), logger).ErrorContext(
						r.Context(),
						"write panic response",
						"error", err,
					)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(production bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

func cors(allowedOrigins []string, logger *slog.Logger, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if _, ok := allowed[origin]; !ok {
			writeMappedError(
				w,
				r,
				logger,
				&APIError{
					Status:  http.StatusForbidden,
					Code:    "ORIGIN_NOT_ALLOWED",
					Message: "Request origin is not allowed.",
				},
			)
			return
		}

		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set(
				"Access-Control-Allow-Headers",
				"Content-Type, Idempotency-Key, X-Requested-With, X-CSRF-Token",
			)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
