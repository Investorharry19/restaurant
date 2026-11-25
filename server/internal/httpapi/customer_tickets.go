package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"restaurant/server/internal/realtime"
)

const customerRecordTTL = 24 * time.Hour

var (
	customerNamePattern = regexp.MustCompile(`\s+`)
	pickupInputPattern  = regexp.MustCompile(`^[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{8}$`)
)

type publicTicketController struct {
	db       *sql.DB
	logger   *slog.Logger
	limiter  *rateLimiter
	eventHub *realtime.Hub
}

type createCustomerTicketRequest struct {
	Type        string `json:"type"`
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	PartySize   int    `json:"partySize"`
	PickupCode  string `json:"pickupCode"`
}

type createCustomerTicketResponse struct {
	PublicToken  string    `json:"publicToken"`
	TicketNumber int64     `json:"ticketNumber"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type publicTicketStatus struct {
	RestaurantName  string    `json:"restaurantName"`
	TicketNumber    int64     `json:"ticketNumber"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	PartySize       *int      `json:"partySize"`
	QueuePosition   *int64    `json:"queuePosition"`
	PositionMessage string    `json:"positionMessage"`
	NextStep        string    `json:"nextStep"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func newPublicTicketController(db *sql.DB, logger *slog.Logger, eventHub *realtime.Hub) *publicTicketController {
	return &publicTicketController{
		db:       db,
		logger:   logger,
		limiter:  newRateLimiter(12, time.Minute),
		eventHub: eventHub,
	}
}

func registerCustomerTicketRoutes(mux *http.ServeMux, controller *publicTicketController) {
	mux.HandleFunc(
		"POST /api/public/restaurants/{slug}/queue-tickets",
		controller.createTicket,
	)
	mux.HandleFunc("GET /api/public/tickets/{publicToken}", controller.getTicket)
	mux.HandleFunc("GET /api/public/tickets/{publicToken}/events", controller.ticketEvents)
}

func (controller *publicTicketController) createTicket(w http.ResponseWriter, r *http.Request) {
	setPrivateTicketHeaders(w)
	if !controller.limiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusTooManyRequests,
			Code:    "QUEUE_RATE_LIMITED",
			Message: "Too many queue requests. Try again later.",
		})
		return
	}
	var body createCustomerTicketRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.DisplayName = strings.TrimSpace(customerNamePattern.ReplaceAllString(body.DisplayName, " "))
	body.Phone = strings.TrimSpace(body.Phone)
	body.PickupCode = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(body.PickupCode), "-", ""))
	if err := validateCustomerTicketRequest(body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin customer ticket: %w", err))
		return
	}
	defer tx.Rollback()

	var restaurantID string
	var timezone string
	var restaurantActive bool
	var tableEnabled bool
	var pickupEnabled bool
	err = tx.QueryRowContext(r.Context(), `
		SELECT id::text, timezone, is_active, table_queue_enabled, pickup_queue_enabled
		FROM restaurants
		WHERE slug = $1
		FOR SHARE
	`, r.PathValue("slug")).Scan(
		&restaurantID,
		&timezone,
		&restaurantActive,
		&tableEnabled,
		&pickupEnabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusNotFound,
			Code:    "RESTAURANT_NOT_FOUND",
			Message: "Restaurant not found.",
		})
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load restaurant for ticket: %w", err))
		return
	}

	typeEnabled := body.Type == "TABLE" && tableEnabled || body.Type == "PICKUP" && pickupEnabled
	if !restaurantActive || !typeEnabled {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusConflict,
			Code:    "QUEUE_UNAVAILABLE",
			Message: "This queue is not available right now.",
		})
		return
	}
	businessDate, err := restaurantBusinessDate(time.Now(), timezone)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate restaurant business date: %w", err))
		return
	}

	var orderID *string
	status := "WAITING"
	if body.Type == "PICKUP" {
		codeHash := sha256.Sum256([]byte(body.PickupCode))
		var id string
		var orderStatus string
		err := tx.QueryRowContext(r.Context(), `
			SELECT id::text, status
			FROM orders
			WHERE restaurant_id = $1 AND pickup_code_hash = $2
			  AND status NOT IN ('PICKED_UP', 'CANCELLED')
			FOR UPDATE
		`, restaurantID, codeHash[:]).Scan(&id, &orderStatus)
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusNotFound,
				Code:    "PICKUP_CODE_INVALID",
				Message: "Pickup code was not recognized.",
			})
			return
		}
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("find pickup order: %w", err))
			return
		}
		orderID = &id
		status = ticketStatusForOrder(orderStatus)
		var existingTicketID string
		err = tx.QueryRowContext(r.Context(), `
			SELECT id::text
			FROM queue_tickets
			WHERE order_id = $1
		`, id).Scan(&existingTicketID)
		if err == nil {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusConflict,
				Code:    "PICKUP_TICKET_EXISTS",
				Message: "A ticket has already been created for this pickup order.",
			})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, fmt.Errorf("check existing pickup ticket: %w", err))
			return
		}
	}

	expiresAt := time.Now().UTC().Add(customerRecordTTL)
	var sessionID string
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO customer_sessions (restaurant_id, display_name, phone, expires_at)
		VALUES ($1, $2, NULLIF($3, ''), $4)
		RETURNING id::text
	`, restaurantID, body.DisplayName, body.Phone, expiresAt).Scan(&sessionID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create customer session: %w", err))
		return
	}

	var queueID string
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO queues (restaurant_id, type, business_date)
		VALUES ($1, $2, $3)
		ON CONFLICT (restaurant_id, type, business_date) DO NOTHING
	`, restaurantID, body.Type, businessDate); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create restaurant queue: %w", err))
		return
	}
	var ticketNumber int64
	err = tx.QueryRowContext(r.Context(), `
		UPDATE queues
		SET next_ticket_number = next_ticket_number + 1
		WHERE restaurant_id = $1 AND type = $2 AND business_date = $3 AND is_open
		RETURNING id::text, next_ticket_number - 1
	`, restaurantID, body.Type, businessDate).Scan(&queueID, &ticketNumber)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusConflict,
			Code:    "QUEUE_CLOSED",
			Message: "This queue is not accepting tickets right now.",
		})
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("allocate ticket number: %w", err))
		return
	}

	tokenString, tokenHash, err := newPublicTicketToken()
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate public ticket token: %w", err))
		return
	}
	var partySize any
	if body.Type == "TABLE" {
		partySize = body.PartySize
	}
	var ticketID string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO queue_tickets (
			restaurant_id, queue_id, customer_session_id, order_id,
			ticket_number, public_token_hash, type, status, party_size, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id::text
	`, restaurantID, queueID, sessionID, orderID, ticketNumber, tokenHash[:],
		body.Type, status, partySize, expiresAt).Scan(&ticketID)
	if err != nil {
		var postgresError *pgconn.PgError
		if body.Type == "PICKUP" &&
			errors.As(err, &postgresError) &&
			postgresError.Code == "23505" {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusConflict,
				Code:    "PICKUP_TICKET_EXISTS",
				Message: "A ticket has already been created for this pickup order.",
			})
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("create queue ticket: %w", err))
		return
	}
	eventType := "queue.ticket.created"
	if body.Type == "PICKUP" {
		eventType = "pickup.ticket.created"
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		restaurantID,
		ticketID,
		eventType,
		map[string]any{
			"ticketNumber": ticketNumber,
			"type":         body.Type,
			"status":       status,
		},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit customer ticket: %w", err))
		return
	}

	response := createCustomerTicketResponse{
		PublicToken:  tokenString,
		TicketNumber: ticketNumber,
		Type:         body.Type,
		Status:       status,
		ExpiresAt:    expiresAt,
	}
	if err := writeSuccess(w, http.StatusCreated, "Queue ticket created.", response); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write queue ticket response",
			"error", err,
		)
	}
}

func validateCustomerTicketRequest(body createCustomerTicketRequest) error {
	if body.Type != "TABLE" && body.Type != "PICKUP" {
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "TICKET_INVALID",
			Message: "Choose a supported queue type.",
		}
	}
	if utf8.RuneCountInString(body.DisplayName) < 1 ||
		utf8.RuneCountInString(body.DisplayName) > 120 ||
		utf8.RuneCountInString(body.Phone) > 50 {
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "TICKET_INVALID",
			Message: "Enter a name of at most 120 characters and a phone number of at most 50 characters.",
		}
	}
	if body.Type == "TABLE" && (body.PartySize < 1 || body.PartySize > 20 || body.PickupCode != "") {
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "TICKET_INVALID",
			Message: "Table wait tickets require a party size from 1 to 20.",
		}
	}
	if body.Type == "PICKUP" && (!pickupInputPattern.MatchString(body.PickupCode) || body.PartySize != 0) {
		return &APIError{
			Status:  http.StatusBadRequest,
			Code:    "TICKET_INVALID",
			Message: "Enter the 8-character pickup code from the restaurant.",
		}
	}
	return nil
}

func (controller *publicTicketController) getTicket(w http.ResponseWriter, r *http.Request) {
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
	var ticket publicTicketStatus
	var partySize sql.NullInt64
	var queuePosition sql.NullInt64
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT r.name, t.ticket_number, t.type,
		       CASE
		         WHEN t.expires_at <= now() AND t.status NOT IN
		           ('SEATED', 'NO_SHOW', 'PICKED_UP', 'CANCELLED', 'EXPIRED')
		         THEN 'EXPIRED'
		         ELSE t.status
		       END,
		       t.party_size,
		       CASE 		       WHEN t.type = 'TABLE' AND t.status = 'WAITING' AND t.expires_at > now()
		         THEN (
		           SELECT count(*) + 1
		           FROM queue_tickets ahead
		           WHERE ahead.queue_id = t.queue_id
		             AND ahead.type = 'TABLE'
		             AND ahead.status = 'WAITING'
		             AND ahead.expires_at > now()
		             AND (ahead.created_at, ahead.id) <= (t.created_at, t.id)
		         )
		         ELSE NULL
		       END,
		       t.created_at, t.updated_at, t.expires_at
		FROM queue_tickets t
		JOIN restaurants r ON r.id = t.restaurant_id
		WHERE t.public_token_hash = $1
	`, tokenHash[:]).Scan(
		&ticket.RestaurantName,
		&ticket.TicketNumber,
		&ticket.Type,
		&ticket.Status,
		&partySize,
		&queuePosition,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
		&ticket.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, ticketNotFound())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load customer ticket: %w", err))
		return
	}
	if partySize.Valid {
		value := int(partySize.Int64)
		ticket.PartySize = &value
	}
	if queuePosition.Valid {
		ticket.QueuePosition = &queuePosition.Int64
	}
	ticket.PositionMessage = ticketPositionMessage(ticket.Type, ticket.Status)
	ticket.NextStep = ticketNextStep(ticket.Type, ticket.Status)
	if err := writeSuccess(w, http.StatusOK, "Ticket loaded.", ticket); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write customer ticket response",
			"error", err,
		)
	}
}

func setPrivateTicketHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}

func newPublicTicketToken() (string, []byte, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", nil, err
	}
	tokenHash := sha256.Sum256(token)
	return base64.RawURLEncoding.EncodeToString(token), tokenHash[:], nil
}

func publicTicketTokenHash(tokenString string) ([32]byte, bool) {
	token, err := base64.RawURLEncoding.DecodeString(tokenString)
	if err != nil || len(token) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(token), true
}

func ticketNotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    "TICKET_NOT_FOUND",
		Message: "Ticket not found or no longer available.",
	}
}

func ticketPositionMessage(queueType, status string) string {
	if queueType == "PICKUP" {
		return "Pickup progress is shown through the order status."
	}
	if status != "WAITING" {
		return "Queue position is shown while the ticket is waiting."
	}
	return ""
}

func ticketNextStep(queueType, status string) string {
	if queueType == "PICKUP" {
		switch status {
		case "WAITING":
			return "Keep this ticket handy and check the order status."
		case "PREPARING":
			return "Your order is being prepared. Check back for updates."
		case "READY":
			return "Your order is ready. Return to the restaurant pickup counter."
		case "PICKED_UP":
			return "This order has been picked up."
		case "CANCELLED":
			return "This pickup ticket was cancelled. Contact the restaurant for help."
		default:
			return "This pickup ticket has expired. Contact the restaurant for help."
		}
	}
	switch status {
	case "WAITING":
		return "Keep this ticket handy and watch for your place in the queue."
	case "CALLED":
		return "The restaurant has called your party. Please return to the host."
	case "ARRIVED":
		return "Your arrival is confirmed. The restaurant will help you next."
	case "SEATED":
		return "Your party has been seated."
	case "CANCELLED":
		return "This ticket was cancelled."
	case "NO_SHOW":
		return "This ticket was marked no-show. Contact the restaurant for help."
	default:
		return "This ticket has expired. Contact the restaurant for help."
	}
}
