package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendVerificationEmailSendsAuthenticatedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Errorf("request = %s %s, want POST /emails", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer re_test_key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		var body resendEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode email request: %v", err)
		}
		if body.From != "Restaurant <verify@example.com>" ||
			len(body.To) != 1 || body.To[0] != "owner@example.com" ||
			body.Subject == "" ||
			!strings.Contains(body.Text, "http://localhost:3000/verify-email?token=abc") ||
			!strings.Contains(body.HTML, "Verify email address") {
			t.Errorf("unexpected Resend payload: %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	sender := &resendVerificationEmailSender{
		apiKey:   "re_test_key",
		from:     "Restaurant <verify@example.com>",
		endpoint: server.URL + "/emails",
		client:   server.Client(),
	}
	if err := sender.SendVerificationEmail(
		context.Background(),
		"owner@example.com",
		"http://localhost:3000/verify-email?token=abc",
	); err != nil {
		t.Fatalf("SendVerificationEmail() error = %v", err)
	}
}

func TestResendVerificationEmailSurfacesProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	sender := &resendVerificationEmailSender{
		apiKey:   "re_test_key",
		from:     "verify@example.com",
		endpoint: server.URL,
		client:   server.Client(),
	}
	if err := sender.SendVerificationEmail(
		context.Background(),
		"owner@example.com",
		"http://localhost:3000/verify-email?token=abc",
	); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("SendVerificationEmail() error = %v, want provider status error", err)
	}
}
