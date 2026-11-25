package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

const resendEmailEndpoint = "https://api.resend.com/emails"

type VerificationEmailSender interface {
	SendVerificationEmail(context.Context, string, string) error
}

type StaffInvitationEmailSender interface {
	SendStaffInvitationEmail(context.Context, string, string, string, string) error
}

type resendVerificationEmailSender struct {
	apiKey   string
	from     string
	endpoint string
	client   *http.Client
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func NewResendVerificationEmailSender(apiKey, from string) VerificationEmailSender {
	return &resendVerificationEmailSender{
		apiKey:   apiKey,
		from:     from,
		endpoint: resendEmailEndpoint,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (sender *resendVerificationEmailSender) SendVerificationEmail(
	ctx context.Context,
	email string,
	verificationURL string,
) error {
	if strings.TrimSpace(sender.apiKey) == "" || strings.TrimSpace(sender.from) == "" {
		return fmt.Errorf("Resend email delivery is not configured")
	}

	return sender.sendEmail(
		ctx,
		email,
		"Verify your restaurant owner account",
		"<p>Confirm your email address to continue with your restaurant application.</p><p><a href=\""+html.EscapeString(verificationURL)+"\">Verify email address</a></p><p>This link expires in 24 hours. If you did not request this, you can ignore this email.</p>",
		"Confirm your email address to continue with your restaurant application:\n\n"+verificationURL+"\n\nThis link expires in 24 hours. If you did not request this, you can ignore this email.",
	)
}

func (sender *resendVerificationEmailSender) SendStaffInvitationEmail(
	ctx context.Context,
	email string,
	invitationURL string,
	restaurantName string,
	role string,
) error {
	if strings.TrimSpace(sender.apiKey) == "" || strings.TrimSpace(sender.from) == "" {
		return fmt.Errorf("Resend email delivery is not configured")
	}
	safeRestaurantName := html.EscapeString(restaurantName)
	safeRole := html.EscapeString(strings.ToLower(role))
	return sender.sendEmail(
		ctx,
		email,
		"You're invited to join "+restaurantName,
		"<p>You have been invited to join <strong>"+safeRestaurantName+"</strong> as a "+safeRole+".</p><p><a href=\""+html.EscapeString(invitationURL)+"\">Accept invitation</a></p><p>This link expires in 72 hours. If you did not expect this invitation, you can ignore this email.</p>",
		"You have been invited to join "+restaurantName+" as a "+strings.ToLower(role)+".\n\nAccept the invitation:\n"+invitationURL+"\n\nThis link expires in 72 hours. If you did not expect this invitation, you can ignore this email.",
	)
}

func (sender *resendVerificationEmailSender) sendEmail(
	ctx context.Context,
	email string,
	subject string,
	htmlBody string,
	textBody string,
) error {
	body, err := json.Marshal(resendEmailRequest{
		From:    sender.from,
		To:      []string{email},
		Subject: subject,
		HTML:    htmlBody,
		Text:    textBody,
	})
	if err != nil {
		return fmt.Errorf("encode Resend email request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		sender.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create Resend email request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+sender.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := sender.client.Do(request)
	if err != nil {
		return fmt.Errorf("send verification email with Resend: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("Resend returned HTTP %d and response could not be read: %w", response.StatusCode, readErr)
		}
		return fmt.Errorf("Resend returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}
