package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/example",
		strings.NewReader(`{"name":"table","unexpected":true}`),
	)
	var body struct {
		Name string `json:"name"`
	}

	err := decodeJSON(httptest.NewRecorder(), request, &body)
	if err == nil {
		t.Fatal("expected unknown JSON field to fail")
	}
}

func TestDecodeJSONRejectsOversizedBodies(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/example",
		strings.NewReader(`{"value":"`+strings.Repeat("a", maxJSONBodyBytes)+`"}`),
	)
	var body struct {
		Value string `json:"value"`
	}

	err := decodeJSON(httptest.NewRecorder(), request, &body)
	if err == nil {
		t.Fatal("expected oversized JSON body to fail")
	}
}

func TestMiddlewareSetsRequestIDAndSecurityHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		logger,
		[]string{"https://restaurant.example"},
		true,
	)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected request ID response header")
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected nosniff security header")
	}
	if recorder.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("expected HSTS in production")
	}
}

func TestCORSMiddlewareRejectsUnlistedOrigins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := cors(
		[]string{"https://restaurant.example"},
		logger,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("handler should not be called for an unlisted origin")
		}),
	)
	request := httptest.NewRequest(http.MethodGet, "/api/example", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if !strings.Contains(recorder.Body.String(), `"code":"ORIGIN_NOT_ALLOWED"`) {
		t.Fatalf("body = %q, want origin error envelope", recorder.Body.String())
	}
}
