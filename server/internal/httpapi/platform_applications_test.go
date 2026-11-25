package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeAndValidateApplication(t *testing.T) {
	body := createBusinessApplicationRequest{
		BusinessName:       "  A Neighborhood Bakery  ",
		BusinessType:       "bakery",
		Slug:               "Neighborhood-Bakery",
		Address:            "  10 Main Street  ",
		Phone:              " 555-0100 ",
		TableQueueEnabled:  true,
		PickupQueueEnabled: true,
	}

	if err := normalizeAndValidateApplication(&body); err != nil {
		t.Fatalf("normalizeAndValidateApplication() error = %v", err)
	}
	if body.BusinessName != "A Neighborhood Bakery" ||
		body.BusinessType != "BAKERY" ||
		body.Slug != "neighborhood-bakery" {
		t.Fatalf("application values were not normalized: %#v", body)
	}
}

func TestNormalizeAndValidateApplicationRejectsInvalidInputs(t *testing.T) {
	validApplication := func() createBusinessApplicationRequest {
		return createBusinessApplicationRequest{
			BusinessName:       "A Neighborhood Bakery",
			BusinessType:       "BAKERY",
			Slug:               "neighborhood-bakery",
			PickupQueueEnabled: true,
		}
	}

	tests := []struct {
		name   string
		change func(*createBusinessApplicationRequest)
	}{
		{
			name: "unsupported business type",
			change: func(body *createBusinessApplicationRequest) {
				body.BusinessType = "GROCERY"
			},
		},
		{
			name: "malformed slug",
			change: func(body *createBusinessApplicationRequest) {
				body.Slug = "bakery--one"
			},
		},
		{
			name: "no queue service",
			change: func(body *createBusinessApplicationRequest) {
				body.PickupQueueEnabled = false
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := validApplication()
			test.change(&body)
			if err := normalizeAndValidateApplication(&body); err == nil {
				t.Fatal("normalizeAndValidateApplication() error = nil, want validation error")
			}
		})
	}
}

func TestApplicationStatusTokenPattern(t *testing.T) {
	valid := strings.Repeat("a", 43)
	if !applicationStatusPattern.MatchString(valid) {
		t.Fatal("application status token pattern rejected a URL-safe 256-bit token")
	}
	for _, invalid := range []string{
		strings.Repeat("a", 42),
		strings.Repeat("a", 44),
		strings.Repeat("a", 42) + ".",
	} {
		if applicationStatusPattern.MatchString(invalid) {
			t.Fatalf("application status token pattern accepted %q", invalid)
		}
	}
}

func TestApplicationSubmissionRequiresVerifiedOwnerSession(t *testing.T) {
	handler := NewHandlerWithEmailSender(
		nil,
		slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
		nil,
		false,
		0,
		"",
		"http://localhost:3000",
		nil,
	)
	request := httptest.NewRequest(http.MethodPost, "/api/owner/applications", strings.NewReader(`{}`))
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("POST /api/owner/applications status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != "OWNER_AUTH_REQUIRED" {
		t.Fatalf("response code = %q, want OWNER_AUTH_REQUIRED", response.Code)
	}
}

func TestPublicApplicationSubmissionRouteIsRemoved(t *testing.T) {
	handler := NewHandlerWithEmailSender(
		nil,
		slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
		nil,
		false,
		0,
		"",
		"http://localhost:3000",
		nil,
	)
	request := httptest.NewRequest(http.MethodPost, "/api/public/applications", strings.NewReader(`{}`))
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("POST /api/public/applications status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
