package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateCustomerTicketRequest(t *testing.T) {
	tests := []struct {
		name    string
		request createCustomerTicketRequest
		wantErr bool
	}{
		{
			name: "table ticket",
			request: createCustomerTicketRequest{
				Type:        "TABLE",
				DisplayName: "Alex",
				PartySize:   4,
			},
		},
		{
			name: "pickup ticket",
			request: createCustomerTicketRequest{
				Type:        "PICKUP",
				DisplayName: "Alex",
				PickupCode:  "23456789",
			},
		},
		{
			name: "party size required",
			request: createCustomerTicketRequest{
				Type:        "TABLE",
				DisplayName: "Alex",
			},
			wantErr: true,
		},
		{
			name: "pickup code required",
			request: createCustomerTicketRequest{
				Type:        "PICKUP",
				DisplayName: "Alex",
				PickupCode:  "123",
			},
			wantErr: true,
		},
		{
			name: "pickup code cannot include party size",
			request: createCustomerTicketRequest{
				Type:        "PICKUP",
				DisplayName: "Alex",
				PartySize:   2,
				PickupCode:  "23456789",
			},
			wantErr: true,
		},
		{
			name: "unsupported queue",
			request: createCustomerTicketRequest{
				Type:        "DELIVERY",
				DisplayName: "Alex",
			},
			wantErr: true,
		},
		{
			name: "display name required",
			request: createCustomerTicketRequest{
				Type:      "TABLE",
				PartySize: 1,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCustomerTicketRequest(test.request)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCustomerTicketRequest() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestTicketStatusInstructions(t *testing.T) {
	if got := ticketNextStep("PICKUP", "READY"); got != "Your order is ready. Return to the restaurant pickup counter." {
		t.Fatalf("pickup ready instructions = %q", got)
	}
	if got := ticketNextStep("TABLE", "CALLED"); got != "The restaurant has called your party. Please return to the host." {
		t.Fatalf("table called instructions = %q", got)
	}
	if got := ticketPositionMessage("PICKUP", "WAITING"); got != "Pickup progress is shown through the order status." {
		t.Fatalf("pickup position message = %q", got)
	}
}

func TestNewPublicTicketTokenHas256URLSafeBits(t *testing.T) {
	token, tokenHash, err := newPublicTicketToken()
	if err != nil {
		t.Fatalf("newPublicTicketToken() error = %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("ticket token is not URL-safe base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("ticket token has %d bytes, want 32", len(decoded))
	}
	if got := sha256.Sum256(decoded); !bytes.Equal(tokenHash, got[:]) {
		t.Fatal("ticket token hash does not match the token")
	}
}

func TestValidatePushSubscriptionKeys(t *testing.T) {
	publicKey := make([]byte, 65)
	publicKey[0] = 4
	authKey := make([]byte, 16)
	valid := pushSubscriptionRequest{Endpoint: "https://push.example.test/send"}
	valid.Keys.P256dh = base64.RawURLEncoding.EncodeToString(publicKey)
	valid.Keys.Auth = base64.RawURLEncoding.EncodeToString(authKey)
	if err := validatePushSubscriptionKeys(valid); err != nil {
		t.Fatalf("validatePushSubscriptionKeys() rejected a valid subscription: %v", err)
	}

	tests := []struct {
		name string
		edit func(*pushSubscriptionRequest)
	}{
		{
			name: "insecure endpoint",
			edit: func(body *pushSubscriptionRequest) {
				body.Endpoint = "http://push.example.test/send"
			},
		},
		{
			name: "nonstandard port",
			edit: func(body *pushSubscriptionRequest) {
				body.Endpoint = "https://push.example.test:8443/send"
			},
		},
		{
			name: "invalid public key",
			edit: func(body *pushSubscriptionRequest) {
				body.Keys.P256dh = base64.RawURLEncoding.EncodeToString([]byte("invalid"))
			},
		},
		{
			name: "invalid auth key",
			edit: func(body *pushSubscriptionRequest) {
				body.Keys.Auth = base64.RawURLEncoding.EncodeToString([]byte("short"))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := valid
			test.edit(&body)
			if err := validatePushSubscriptionKeys(body); err == nil {
				t.Fatal("validatePushSubscriptionKeys() accepted invalid subscription")
			}
		})
	}
}

func TestPushEligibleTicketRequiresActiveUnexpiredStatus(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	active := []string{"WAITING", "CALLED", "ARRIVED", "PREPARING", "READY"}
	for _, status := range active {
		if !pushEligibleTicket(status, now.Add(time.Minute), now) {
			t.Errorf("pushEligibleTicket(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"SEATED", "NO_SHOW", "PICKED_UP", "CANCELLED", "EXPIRED"} {
		if pushEligibleTicket(status, now.Add(time.Minute), now) {
			t.Errorf("pushEligibleTicket(%q) = true, want false", status)
		}
	}
	if pushEligibleTicket("WAITING", now, now) {
		t.Fatal("pushEligibleTicket() accepted an expired ticket")
	}
}

func TestRestaurantBusinessDateUsesRestaurantTimezone(t *testing.T) {
	now := time.Date(2025, time.January, 1, 2, 0, 0, 0, time.UTC)
	got, err := restaurantBusinessDate(now, "America/New_York")
	if err != nil {
		t.Fatalf("restaurantBusinessDate() error = %v", err)
	}
	if got != "2024-12-31" {
		t.Fatalf("business date = %q, want 2024-12-31", got)
	}

	if _, err := restaurantBusinessDate(now, "Invalid/Timezone"); err == nil {
		t.Fatal("restaurantBusinessDate() accepted an invalid timezone")
	}
}

func TestPublicTicketRouteUsesPathToken(t *testing.T) {
	controller := &publicTicketController{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: newRateLimiter(12, 0),
	}
	mux := http.NewServeMux()
	registerCustomerTicketRoutes(mux, controller)

	for _, path := range []string{
		"/api/public/tickets/not-a-token",
		"/api/public/tickets/not-a-token/events",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}
