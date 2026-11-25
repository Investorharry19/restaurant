package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"restaurant/server/internal/realtime"
)

type sseMessage struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

func (controller *ownerWorkspaceController) workspaceEvents(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	subscription := controller.eventHub.SubscribeRestaurant(identity.restaurantID)
	reauthorize := controller.ownerAuth.streamAccessCheck(r, identity)
	if _, ok := staffIdentityFromContext(r.Context()); ok {
		reauthorize = controller.staffAuth.streamAccessCheck(r, identity)
	}
	streamEvents(w, r, controller.logger, subscription, reauthorize)
}

func (controller *ownerWorkspaceController) staffEvents(w http.ResponseWriter, r *http.Request) {
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	subscription := controller.eventHub.SubscribeRestaurant(identity.restaurantID)
	streamEvents(
		w,
		r,
		controller.logger,
		subscription,
		controller.staffAuth.streamAccessCheck(r, identity),
	)
}

func (controller *publicTicketController) ticketEvents(w http.ResponseWriter, r *http.Request) {
	setPrivateTicketHeaders(w)
	if !controller.limiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusTooManyRequests,
			Code:    "TICKET_RATE_LIMITED",
			Message: "Too many ticket lookups. Try again later.",
		})
		return
	}
	tokenHash, ok := publicTicketTokenHash(r.PathValue("publicToken"))
	if !ok {
		writeMappedError(w, r, controller.logger, ticketNotFound())
		return
	}
	var restaurantID string
	var ticketID string
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT restaurant_id::text, id::text
		FROM queue_tickets
		WHERE public_token_hash = $1
	`, tokenHash[:]).Scan(&restaurantID, &ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, ticketNotFound())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("authorize customer ticket event stream: %w", err))
		return
	}
	subscription := controller.eventHub.SubscribeTicket(restaurantID, ticketID)
	streamEvents(w, r, controller.logger, subscription, nil)
}

type streamAccessCheck func(context.Context) (bool, error)

func (controller *staffAuthController) streamAccessCheck(
	r *http.Request,
	identity staffIdentity,
) streamAccessCheck {
	return func(ctx context.Context) (bool, error) {
		tokenHash, err := staffSessionHashFromRequest(r)
		if err != nil {
			return false, nil
		}
		var active bool
		if err := controller.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM staff_sessions s
				JOIN users u ON u.id = s.user_id AND u.is_active
				JOIN restaurant_memberships m
				  ON m.user_id = u.id AND m.restaurant_id = s.active_restaurant_id AND m.is_active
				JOIN restaurants restaurant
				  ON restaurant.id = m.restaurant_id AND restaurant.is_active
				WHERE s.session_token_hash = $1 AND s.user_id = $2
				  AND s.active_restaurant_id = $3 AND s.expires_at > now()
			)
		`, tokenHash, identity.userID, identity.restaurantID).Scan(&active); err != nil {
			return false, fmt.Errorf("recheck staff event stream authorization: %w", err)
		}
		return active, nil
	}
}

func (controller *ownerAuthController) streamAccessCheck(
	r *http.Request,
	identity staffIdentity,
) streamAccessCheck {
	return func(ctx context.Context) (bool, error) {
		cookie, err := r.Cookie(ownerSessionCookieName)
		if err != nil {
			return false, nil
		}
		token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		if err != nil || len(token) != 32 {
			clear(token)
			return false, nil
		}
		tokenHash := sha256.Sum256(token)
		clear(token)

		var active bool
		if err := controller.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM owner_sessions s
				JOIN users u ON u.id = s.user_id
				  AND u.is_active AND u.email_verified_at IS NOT NULL
				JOIN restaurant_memberships m
				  ON m.user_id = u.id AND m.restaurant_id = $2 AND m.is_active
				JOIN restaurants restaurant
				  ON restaurant.id = m.restaurant_id AND restaurant.is_active
				WHERE s.session_token_hash = $1 AND s.user_id = $3 AND s.expires_at > now()
			)
		`, tokenHash[:], identity.restaurantID, identity.userID).Scan(&active); err != nil {
			return false, fmt.Errorf("recheck owner event stream authorization: %w", err)
		}
		return active, nil
	}
}

func streamEvents(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	subscription *realtime.Subscription,
	reauthorize streamAccessCheck,
) {
	defer subscription.Close()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	controller := http.NewResponseController(w)
	if err := writeSSEComment(controller, w, "connected"); err != nil {
		if !errors.Is(err, context.Canceled) {
			loggerFromContext(r.Context(), logger).ErrorContext(
				r.Context(), "start realtime event stream", "error", err,
			)
		}
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-subscription.Events():
			if !open {
				return
			}
			message, err := json.Marshal(sseMessage{
				Type:      event.Type,
				Timestamp: event.Timestamp,
				Data:      event.Data,
			})
			if err != nil {
				loggerFromContext(r.Context(), logger).ErrorContext(
					r.Context(), "encode realtime event", "error", err,
				)
				return
			}
			if err := writeSSEEvent(controller, w, event.ID, message); err != nil {
				if !errors.Is(err, context.Canceled) {
					loggerFromContext(r.Context(), logger).ErrorContext(
						r.Context(), "write realtime event", "error", err,
					)
				}
				return
			}
		case <-heartbeat.C:
			if reauthorize != nil {
				active, err := reauthorize(r.Context())
				if err != nil {
					loggerFromContext(r.Context(), logger).ErrorContext(
						r.Context(), "recheck realtime stream authorization", "error", err,
					)
					return
				}
				if !active {
					if err := writeSSEControlEvent(controller, w, "session.expired"); err != nil {
						loggerFromContext(r.Context(), logger).ErrorContext(
							r.Context(), "close unauthorized realtime stream", "error", err,
						)
					}
					return
				}
			}
			if err := writeSSEComment(controller, w, "heartbeat"); err != nil {
				if !errors.Is(err, context.Canceled) {
					loggerFromContext(r.Context(), logger).ErrorContext(
						r.Context(), "write realtime heartbeat", "error", err,
					)
				}
				return
			}
		}
	}
}

func writeSSEControlEvent(
	controller *http.ResponseController,
	w http.ResponseWriter,
	eventType string,
) error {
	if err := controller.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("set realtime stream write deadline: %w", err)
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: {}\n\n", eventType); err != nil {
		return fmt.Errorf("write realtime stream control event: %w", err)
	}
	if err := controller.Flush(); err != nil {
		return fmt.Errorf("flush realtime stream control event: %w", err)
	}
	return nil
}

func writeSSEComment(
	controller *http.ResponseController,
	w http.ResponseWriter,
	comment string,
) error {
	if err := controller.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("set realtime stream write deadline: %w", err)
	}
	if _, err := fmt.Fprintf(w, ": %s\n\n", comment); err != nil {
		return fmt.Errorf("write realtime stream comment: %w", err)
	}
	if err := controller.Flush(); err != nil {
		return fmt.Errorf("flush realtime stream comment: %w", err)
	}
	return nil
}

func writeSSEEvent(
	controller *http.ResponseController,
	w http.ResponseWriter,
	id string,
	message []byte,
) error {
	if err := controller.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("set realtime stream write deadline: %w", err)
	}
	if _, err := fmt.Fprintf(w, "id: %s\ndata: %s\n\n", id, message); err != nil {
		return fmt.Errorf("write realtime event frame: %w", err)
	}
	if err := controller.Flush(); err != nil {
		return fmt.Errorf("flush realtime event: %w", err)
	}
	return nil
}
