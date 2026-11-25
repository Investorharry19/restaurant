package httpapi

import "testing"

func TestValidTableQueueTransition(t *testing.T) {
	tests := []struct {
		current string
		next    string
		allowed bool
	}{
		{current: "WAITING", next: "CALLED", allowed: true},
		{current: "WAITING", next: "CANCELLED", allowed: true},
		{current: "CALLED", next: "ARRIVED", allowed: true},
		{current: "CALLED", next: "NO_SHOW", allowed: true},
		{current: "ARRIVED", next: "CANCELLED", allowed: true},
		{current: "WAITING", next: "ARRIVED", allowed: false},
		{current: "ARRIVED", next: "CALLED", allowed: false},
		{current: "SEATED", next: "NO_SHOW", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.current+" to "+test.next, func(t *testing.T) {
			if got := validTableQueueTransition(test.current, test.next); got != test.allowed {
				t.Fatalf("validTableQueueTransition(%q, %q) = %t, want %t",
					test.current, test.next, got, test.allowed)
			}
		})
	}
}
