package httpapi

import (
	"context"
	"testing"
)

type verificationEmailSenderFunc func(context.Context, string, string) error

func (send verificationEmailSenderFunc) SendVerificationEmail(
	ctx context.Context,
	email string,
	verifyURL string,
) error {
	return send(ctx, email, verifyURL)
}

func TestValidateOwnerRegistration(t *testing.T) {
	valid := ownerRegistrationRequest{
		Name:     "Alex Owner",
		Email:    "alex@example.com",
		Password: "a sufficiently long password",
	}
	if err := validateOwnerRegistration(valid); err != nil {
		t.Fatalf("validateOwnerRegistration() error = %v", err)
	}

	tests := []struct {
		name string
		body ownerRegistrationRequest
	}{
		{
			name: "missing name",
			body: ownerRegistrationRequest{
				Email:    valid.Email,
				Password: valid.Password,
			},
		},
		{
			name: "invalid email",
			body: ownerRegistrationRequest{
				Name:     valid.Name,
				Email:    "not-an-email",
				Password: valid.Password,
			},
		},
		{
			name: "short password",
			body: ownerRegistrationRequest{
				Name:     valid.Name,
				Email:    valid.Email,
				Password: "short",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateOwnerRegistration(test.body); err == nil {
				t.Fatal("validateOwnerRegistration() error = nil, want validation error")
			}
		})
	}
}

func TestSendVerificationEmailUsesConfiguredApplicationOrigin(t *testing.T) {
	controller := &ownerAuthController{
		appBaseURL: "https://restaurant.example",
		emailSender: verificationEmailSenderFunc(func(
			_ context.Context,
			email string,
			verifyURL string,
		) error {
			if email != "owner@example.com" {
				t.Errorf("email = %q", email)
			}
			if verifyURL != "https://restaurant.example/verify-email?token=abc" {
				t.Errorf("verification URL = %q", verifyURL)
			}
			return nil
		}),
	}
	if err := controller.sendVerificationEmail(
		context.Background(),
		"owner@example.com",
		"abc",
	); err != nil {
		t.Fatalf("sendVerificationEmail() error = %v", err)
	}
}
