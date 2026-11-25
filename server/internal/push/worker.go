package push

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	maxAttempts  = 6
	batchSize    = 4
	workerPoll   = time.Second
	jobLeaseTime = time.Minute
)

type Settings struct {
	AppBaseURL         string
	VAPIDPublicKey     string
	VAPIDPrivateKey    string
	VAPIDSubject       string
	TokenEncryptionKey []byte
}

type Worker struct {
	db     *sql.DB
	logger *slog.Logger
	config Settings
	client *http.Client
}

type job struct {
	ID                   string
	RestaurantID         string
	TicketID             string
	LockedBy             string
	Type                 string
	Title                string
	Body                 string
	IdempotencyKey       string
	EncryptedTicketToken []byte
	Attempt              int
}

type target struct {
	ID       string
	Endpoint string
	P256dh   string
	Auth     string
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

func NewWorker(db *sql.DB, logger *slog.Logger, settings Settings) *Worker {
	return &Worker{
		db:     db,
		logger: logger,
		config: settings,
		client: NewPublicEndpointHTTPClientWithResolver(nil),
	}
}

func (worker *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(workerPoll)
	defer ticker.Stop()

	for {
		claimed, err := worker.claim(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			worker.logger.ErrorContext(ctx, "claim push notification jobs", "error", err)
		}
		var deliveries sync.WaitGroup
		for _, current := range claimed {
			if ctx.Err() != nil {
				break
			}
			deliveries.Add(1)
			go func(current job) {
				defer deliveries.Done()
				if err := worker.deliver(ctx, current); err != nil {
					worker.logger.ErrorContext(
						ctx,
						"process push notification job",
						"notification_id", current.ID,
						"error", err,
					)
				}
			}(current)
		}
		deliveries.Wait()
		if ctx.Err() != nil {
			if err := worker.releaseClaimed(claimed); err != nil {
				worker.logger.Error("release push notification leases during shutdown", "error", err)
			}
			return
		}
		if len(claimed) > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (worker *Worker) releaseClaimed(claimed []job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, current := range claimed {
		if _, err := worker.db.ExecContext(ctx, `
			UPDATE notifications
			SET status = 'PENDING', next_attempt_at = now(),
			    locked_at = NULL, locked_by = NULL, updated_at = now()
			WHERE id = $1 AND status = 'PROCESSING' AND locked_by = $2
		`, current.ID, current.LockedBy); err != nil {
			return fmt.Errorf("release push notification %s: %w", current.ID, err)
		}
	}
	return nil
}

func (worker *Worker) claim(ctx context.Context) ([]job, error) {
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin push notification claim: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE notifications
		SET status = 'FAILED', locked_at = NULL, locked_by = NULL,
		    last_error_code = 'RETRY_LIMIT', updated_at = now()
		WHERE status = 'PROCESSING'
		  AND locked_at < now() - ($1 * interval '1 second')
		  AND attempt_count >= $2
	`, int(jobLeaseTime.Seconds()), maxAttempts); err != nil {
		return nil, fmt.Errorf("expire abandoned push notification jobs: %w", err)
	}

	instanceID, err := workerID()
	if err != nil {
		return nil, fmt.Errorf("generate push worker ID: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		WITH due AS (
			SELECT id
			FROM notifications
			WHERE attempt_count < $1
			  AND (
			    (status = 'PENDING' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
			    OR (status = 'PROCESSING' AND locked_at < now() - ($2 * interval '1 second'))
			  )
			ORDER BY COALESCE(next_attempt_at, created_at), created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE notifications n
		SET status = 'PROCESSING',
		    locked_at = now(),
		    locked_by = $4,
		    attempt_count = n.attempt_count + 1,
		    updated_at = now()
		FROM due
		WHERE n.id = due.id
		RETURNING n.id::text, n.restaurant_id::text, COALESCE(n.queue_ticket_id::text, ''),
		          n.locked_by,
		          n.type, n.title, n.body, n.idempotency_key,
		          n.ticket_token_ciphertext, n.attempt_count
	`, maxAttempts, int(jobLeaseTime.Seconds()), batchSize, instanceID)
	if err != nil {
		return nil, fmt.Errorf("claim due push notification jobs: %w", err)
	}
	defer rows.Close()

	claimed := make([]job, 0)
	for rows.Next() {
		var current job
		if err := rows.Scan(
			&current.ID,
			&current.RestaurantID,
			&current.TicketID,
			&current.LockedBy,
			&current.Type,
			&current.Title,
			&current.Body,
			&current.IdempotencyKey,
			&current.EncryptedTicketToken,
			&current.Attempt,
		); err != nil {
			return nil, fmt.Errorf("scan claimed push notification: %w", err)
		}
		claimed = append(claimed, current)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed push notifications: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit push notification claim: %w", err)
	}
	return claimed, nil
}

func (worker *Worker) deliver(ctx context.Context, current job) error {
	token, err := DecryptTicketToken(worker.config.TokenEncryptionKey, current.EncryptedTicketToken)
	if err != nil {
		return worker.finishWithoutRetry(ctx, current, "TICKET_LINK_INVALID")
	}
	if strings.Contains(token, "/") || strings.Contains(token, "\\") {
		return worker.finishWithoutRetry(ctx, current, "TICKET_LINK_INVALID")
	}
	ticketURL := strings.TrimRight(worker.config.AppBaseURL, "/") + "/t/" + token
	payload, err := json.Marshal(pushPayload{
		Title: current.Title,
		Body:  current.Body,
		URL:   ticketURL,
		Tag:   current.IdempotencyKey,
	})
	if err != nil {
		return worker.finishWithoutRetry(ctx, current, "PAYLOAD_INVALID")
	}

	targets, err := worker.targets(ctx, current)
	if err != nil {
		return fmt.Errorf("load push notification targets: %w", err)
	}
	if len(targets) == 0 {
		return worker.finishWithoutRetry(ctx, current, "NO_ACTIVE_SUBSCRIPTIONS")
	}

	type sendResult struct {
		subscription target
		attempt      deliveryAttempt
	}
	results := make(chan sendResult, len(targets))
	for _, subscription := range targets {
		go func(subscription target) {
			results <- sendResult{
				subscription: subscription,
				attempt: worker.send(
					ctx,
					payload,
					subscription,
					current.IdempotencyKey,
				),
			}
		}(subscription)
	}

	result := dispatchResult{}
	attempts := make([]sendResult, 0, len(targets))
	for range targets {
		attempts = append(attempts, <-results)
	}
	for _, sent := range attempts {
		if err := worker.recordAttempt(
			ctx,
			current,
			sent.subscription,
			sent.attempt,
		); err != nil {
			return fmt.Errorf("record push delivery attempt: %w", err)
		}
		attempt := sent.attempt
		switch attempt.outcome {
		case "SENT":
			result.sent++
		case "RETRY":
			result.retry++
			result.errorCode = attempt.errorCode
		case "INVALID":
			result.invalid++
			result.errorCode = attempt.errorCode
		case "FAILED":
			result.failed++
			result.errorCode = attempt.errorCode
		}
	}

	switch {
	case result.retry > 0 && current.Attempt < maxAttempts:
		return worker.scheduleRetry(ctx, current, result.errorCode)
	case result.sent > 0:
		return worker.finish(ctx, current, "SENT", nil)
	case result.retry > 0:
		return worker.finish(ctx, current, "FAILED", stringPointer("RETRY_LIMIT"))
	case result.invalid > 0:
		return worker.finish(ctx, current, "FAILED", stringPointer("SUBSCRIPTION_EXPIRED"))
	default:
		return worker.finish(ctx, current, "FAILED", stringPointer(result.errorCode))
	}
}

type dispatchResult struct {
	sent      int
	retry     int
	invalid   int
	failed    int
	errorCode string
}

type deliveryAttempt struct {
	outcome        string
	responseStatus sql.NullInt64
	errorCode      string
}

func (worker *Worker) targets(ctx context.Context, current job) ([]target, error) {
	rows, err := worker.db.QueryContext(ctx, `
		SELECT s.id::text, s.endpoint, s.p256dh, s.auth
		FROM push_subscriptions s
		WHERE s.queue_ticket_id = $1 AND s.restaurant_id = $2 AND s.is_active
		  AND NOT EXISTS (
		    SELECT 1
		    FROM notification_attempts a
		    WHERE a.notification_id = $3
		      AND a.push_subscription_id = s.id
		      AND a.outcome = 'SENT'
		  )
		ORDER BY s.created_at, s.id
	`, current.TicketID, current.RestaurantID, current.ID)
	if err != nil {
		return nil, fmt.Errorf("query active push subscriptions: %w", err)
	}
	defer rows.Close()

	targets := make([]target, 0)
	for rows.Next() {
		var subscription target
		if err := rows.Scan(
			&subscription.ID,
			&subscription.Endpoint,
			&subscription.P256dh,
			&subscription.Auth,
		); err != nil {
			return nil, fmt.Errorf("scan active push subscription: %w", err)
		}
		targets = append(targets, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read active push subscriptions: %w", err)
	}
	return targets, nil
}

func (worker *Worker) send(
	ctx context.Context,
	payload []byte,
	subscription target,
	idempotencyKey string,
) deliveryAttempt {
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	endpoint, err := ValidateEndpoint(requestCtx, subscription.Endpoint, nil)
	if err != nil {
		if errors.Is(err, ErrUnsafeEndpoint) || errors.Is(err, ErrInvalidEndpoint) {
			return deliveryAttempt{outcome: "INVALID", errorCode: "ENDPOINT_REJECTED"}
		}
		return deliveryAttempt{outcome: "RETRY", errorCode: "ENDPOINT_RESOLUTION_FAILED"}
	}

	webResponse, err := webpush.SendNotificationWithContext(
		requestCtx,
		payload,
		&webpush.Subscription{
			Endpoint: endpoint.String(),
			Keys: webpush.Keys{
				P256dh: subscription.P256dh,
				Auth:   subscription.Auth,
			},
		},
		&webpush.Options{
			Subscriber:      worker.config.VAPIDSubject,
			VAPIDPublicKey:  worker.config.VAPIDPublicKey,
			VAPIDPrivateKey: worker.config.VAPIDPrivateKey,
			TTL:             3600,
			Urgency:         webpush.UrgencyHigh,
			Topic:           topicFor(idempotencyKey),
			HTTPClient:      worker.client,
		},
	)
	if err != nil {
		return deliveryAttempt{outcome: "RETRY", errorCode: "PROVIDER_REQUEST_FAILED"}
	}
	defer webResponse.Body.Close()

	attempt := deliveryAttempt{responseStatus: sql.NullInt64{Int64: int64(webResponse.StatusCode), Valid: true}}
	switch {
	case webResponse.StatusCode >= 200 && webResponse.StatusCode < 300:
		attempt.outcome = "SENT"
	case webResponse.StatusCode == http.StatusGone || webResponse.StatusCode == http.StatusNotFound:
		attempt.outcome = "INVALID"
		attempt.errorCode = "SUBSCRIPTION_EXPIRED"
	case webResponse.StatusCode == http.StatusTooManyRequests || webResponse.StatusCode >= 500:
		attempt.outcome = "RETRY"
		attempt.errorCode = "PROVIDER_TEMPORARY_FAILURE"
	default:
		attempt.outcome = "FAILED"
		attempt.errorCode = "PROVIDER_REJECTED"
	}
	return attempt
}

func (worker *Worker) recordAttempt(
	ctx context.Context,
	current job,
	subscription target,
	attempt deliveryAttempt,
) error {
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin push attempt record: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO notification_attempts (
			notification_id, push_subscription_id, attempt_number,
			outcome, response_status, error_code
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
	`, current.ID, subscription.ID, current.Attempt, attempt.outcome,
		attempt.responseStatus, attempt.errorCode); err != nil {
		return fmt.Errorf("insert notification attempt: %w", err)
	}
	if attempt.outcome == "INVALID" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE push_subscriptions
			SET is_active = false, updated_at = now()
			WHERE id = $1
		`, subscription.ID); err != nil {
			return fmt.Errorf("deactivate invalid push subscription: %w", err)
		}
	}
	if attempt.outcome == "SENT" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE push_subscriptions
			SET last_used_at = now(), updated_at = now()
			WHERE id = $1
		`, subscription.ID); err != nil {
			return fmt.Errorf("update push subscription usage: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit notification attempt: %w", err)
	}
	return nil
}

func (worker *Worker) scheduleRetry(ctx context.Context, current job, errorCode string) error {
	delay := retryDelay(current.Attempt)
	result, err := worker.db.ExecContext(ctx, `
		UPDATE notifications
		SET status = 'PENDING',
		    next_attempt_at = now() + ($1 * interval '1 second'),
		    locked_at = NULL, locked_by = NULL,
		    last_error_code = $2, updated_at = now()
		WHERE id = $3 AND status = 'PROCESSING' AND locked_by = $4
	`, delay.Seconds(), errorCode, current.ID, current.LockedBy)
	if err != nil {
		return fmt.Errorf("schedule push notification retry: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check scheduled push notification retry: %w", err)
	} else if affected != 1 {
		return fmt.Errorf("push notification retry lease is no longer held")
	}
	return nil
}

func (worker *Worker) finishWithoutRetry(ctx context.Context, current job, errorCode string) error {
	status := "FAILED"
	if errorCode == "NO_ACTIVE_SUBSCRIPTIONS" {
		status = "CANCELLED"
	}
	return worker.finish(ctx, current, status, stringPointer(errorCode))
}

func (worker *Worker) finish(
	ctx context.Context,
	current job,
	status string,
	errorCode *string,
) error {
	var lastError any
	if errorCode != nil {
		lastError = *errorCode
	}
	result, err := worker.db.ExecContext(ctx, `
		UPDATE notifications
		SET status = $1,
		    sent_at = CASE WHEN $1 = 'SENT' THEN now() ELSE sent_at END,
		    next_attempt_at = NULL,
		    locked_at = NULL, locked_by = NULL,
		    last_error_code = $2, updated_at = now()
		WHERE id = $3 AND status = 'PROCESSING' AND locked_by = $4
	`, status, lastError, current.ID, current.LockedBy)
	if err != nil {
		return fmt.Errorf("finish push notification job: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("check finished push notification job: %w", err)
	} else if affected != 1 {
		return fmt.Errorf("push notification lease is no longer held")
	}
	return nil
}

func workerID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "push-" + hex.EncodeToString(random), nil
}

func retryDelay(attempt int) time.Duration {
	exponent := attempt - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent > 8 {
		exponent = 8
	}
	base := 15 * time.Second * time.Duration(1<<exponent)
	if base > time.Hour {
		base = time.Hour
	}
	jitterRange := int64(base / 5)
	if jitterRange <= 0 {
		return base
	}
	jitter := time.Duration(mathrand.Int63n(2*jitterRange+1) - jitterRange)
	return base + jitter
}

func topicFor(idempotencyKey string) string {
	digest := sha256.Sum256([]byte(idempotencyKey))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

func stringPointer(value string) *string {
	return &value
}
