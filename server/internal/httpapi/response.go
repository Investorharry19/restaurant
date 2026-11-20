package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const maxJSONBodyBytes = 1 << 20

type successEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type errorEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Data    any    `json:"data"`
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Cause   error
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *APIError) Unwrap() error {
	return e.Cause
}

func writeSuccess(w http.ResponseWriter, status int, message string, data any) error {
	return writeJSON(w, status, successEnvelope{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) error {
	return writeJSON(w, status, errorEnvelope{
		Success: false,
		Message: message,
		Code:    code,
		Data:    nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return fmt.Errorf("encode API response: %w", err)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return &APIError{
				Status:  http.StatusRequestEntityTooLarge,
				Code:    "REQUEST_BODY_TOO_LARGE",
				Message: "Request body is too large.",
				Cause:   err,
			}
		}
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_REQUEST",
			Message: "Request body must contain valid JSON with only supported fields.",
			Cause:   err,
		}
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return &APIError{
				Status:  http.StatusBadRequest,
				Code:    "INVALID_REQUEST",
				Message: "Request body must contain a single JSON value.",
			}
		}
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_REQUEST",
			Message: "Request body must contain a single valid JSON value.",
			Cause:   err,
		}
	}
	return nil
}

func writeMappedError(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	err error,
) {
	status, code, message := mapError(err)
	if status >= http.StatusInternalServerError {
		loggerFromContext(r.Context(), logger).ErrorContext(r.Context(), "API request failed", "error", err)
	}
	if writeErr := writeError(w, status, code, message); writeErr != nil {
		loggerFromContext(r.Context(), logger).ErrorContext(
			r.Context(),
			"write API error response",
			"error", writeErr,
		)
	}
}

func mapError(err error) (int, string, string) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status, apiErr.Code, apiErr.Message
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred."
}
