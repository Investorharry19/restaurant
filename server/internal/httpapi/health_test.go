package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeReadinessChecker struct {
	err error
}

func (f fakeReadinessChecker) PingContext(context.Context) error {
	return f.err
}

func TestHealthHandlerReturnsStandardSuccessEnvelope(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := healthHandler(logger)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"success":true`) {
		t.Fatalf("body = %q, want success envelope", recorder.Body.String())
	}
}

func TestReadinessHandlerDoesNotExposeDatabaseErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := readinessHandler(logger, fakeReadinessChecker{
		err: errors.New("private database connection detail"),
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), "private database connection detail") {
		t.Fatalf("response leaked internal database error: %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"SERVICE_NOT_READY"`) {
		t.Fatalf("body = %q, want stable readiness error code", recorder.Body.String())
	}
}

func TestUnknownAPIRoutesUseTheStandardErrorEnvelope(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(nil, logger, nil, false)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/missing", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("body = %q, want standard API error envelope", recorder.Body.String())
	}
}
