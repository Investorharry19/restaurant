package httpapi

import (
	"strings"
	"testing"
	"time"
)

func TestOrderStateTransitions(t *testing.T) {
	tests := []struct {
		current string
		next    string
		allowed bool
	}{
		{current: "PENDING", next: "ACCEPTED", allowed: true},
		{current: "PENDING", next: "CANCELLED", allowed: true},
		{current: "ACCEPTED", next: "PREPARING", allowed: true},
		{current: "PREPARING", next: "READY", allowed: true},
		{current: "READY", next: "PICKED_UP", allowed: true},
		{current: "PENDING", next: "READY", allowed: false},
		{current: "PICKED_UP", next: "PREPARING", allowed: false},
		{current: "CANCELLED", next: "ACCEPTED", allowed: false},
		{current: "READY", next: "READY", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.current+" to "+test.next, func(t *testing.T) {
			if got := validOrderTransition(test.current, test.next); got != test.allowed {
				t.Fatalf("validOrderTransition(%q, %q) = %t, want %t",
					test.current, test.next, got, test.allowed)
			}
		})
	}
}

func TestRandomPickupCodeIsHumanReadableAndValid(t *testing.T) {
	code, err := randomPickupCode()
	if err != nil {
		t.Fatalf("randomPickupCode() error = %v", err)
	}
	if !pickupInputPattern.MatchString(code) {
		t.Fatalf("pickup code %q does not match the accepted format", code)
	}
}

func TestRateLimiterRejectsAfterConfiguredLimit(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute)
	if !limiter.allow("client") || !limiter.allow("client") {
		t.Fatal("rate limiter rejected a request below its limit")
	}
	if limiter.allow("client") {
		t.Fatal("rate limiter accepted a request over its limit")
	}
	limiter.reset("client")
	if !limiter.allow("client") {
		t.Fatal("rate limiter did not reset the client")
	}
}

func TestOrderIDPatternRejectsMalformedIdentifiers(t *testing.T) {
	for _, value := range []string{
		"not-a-uuid",
		"00000000-0000-0000-0000-000000000000",
		strings.Repeat("a", 36),
	} {
		if orderIDPattern.MatchString(value) {
			t.Fatalf("order ID pattern accepted %q", value)
		}
	}
	valid := "b6c23bd3-96e4-4cd7-8a03-df39c3894e09"
	if !orderIDPattern.MatchString(valid) {
		t.Fatalf("order ID pattern rejected UUID %q", valid)
	}
}
