package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const pickupCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

var orderIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type createOrderRequest struct {
	Notes string `json:"notes"`
}

type createOrderResponse struct {
	ID          string    `json:"id"`
	OrderNumber int64     `json:"orderNumber"`
	PickupCode  string    `json:"pickupCode"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type staffOrderSummary struct {
	ID          string    `json:"id"`
	OrderNumber int64     `json:"orderNumber"`
	Status      string    `json:"status"`
	Notes       *string   `json:"notes"`
	CreatedAt   time.Time `json:"createdAt"`
}

type updateOrderStatusRequest struct {
	Status string `json:"status"`
}

func registerStaffOrderRoutes(mux *http.ServeMux, auth *staffAuthController) {
	mux.Handle("GET /api/staff/orders", auth.requireStaff(http.HandlerFunc(auth.listOrders)))
	mux.Handle("POST /api/staff/orders", auth.requireStaff(http.HandlerFunc(auth.createOrder)))
	mux.Handle(
		"PATCH /api/staff/orders/{orderID}/status",
		auth.requireStaff(http.HandlerFunc(auth.updateOrderStatus)),
	)
}

func (controller *staffAuthController) listOrders(w http.ResponseWriter, r *http.Request) {
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}

	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT id::text, order_number, status, notes, created_at
		FROM orders
		WHERE restaurant_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant orders: %w", err))
		return
	}
	defer rows.Close()

	orders := make([]staffOrderSummary, 0)
	for rows.Next() {
		var order staffOrderSummary
		var notes sql.NullString
		if err := rows.Scan(
			&order.ID,
			&order.OrderNumber,
			&order.Status,
			&notes,
			&order.CreatedAt,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant order: %w", err))
			return
		}
		if notes.Valid {
			order.Notes = &notes.String
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant orders: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Orders loaded.", orders); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write staff orders response",
			"error", err,
		)
	}
}

func (controller *staffAuthController) createOrder(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	var body createOrderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Notes = strings.TrimSpace(body.Notes)
	if len(body.Notes) > 2000 {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "ORDER_INVALID",
			Message: "Order notes must be 2000 characters or fewer.",
		})
		return
	}

	pickupCode, err := randomPickupCode()
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate pickup code: %w", err))
		return
	}
	pickupCodeHash := sha256.Sum256([]byte(pickupCode))
	var timezone string
	if err := controller.db.QueryRowContext(r.Context(), `
		SELECT timezone
		FROM restaurants
		WHERE id = $1 AND is_active
	`, identity.restaurantID).Scan(&timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusNotFound,
				Code:    "RESTAURANT_NOT_FOUND",
				Message: "Restaurant not found.",
			})
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load restaurant timezone: %w", err))
		return
	}
	businessDate, err := restaurantBusinessDate(time.Now(), timezone)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate restaurant business date: %w", err))
		return
	}
	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin order creation: %w", err))
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO restaurant_order_counters (restaurant_id, business_date, next_order_number)
		VALUES ($1, $2, 1)
		ON CONFLICT (restaurant_id, business_date) DO NOTHING
	`, identity.restaurantID, businessDate); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("initialize order counter: %w", err))
		return
	}
	var orderNumber int64
	if err := tx.QueryRowContext(r.Context(), `
		UPDATE restaurant_order_counters
		SET next_order_number = next_order_number + 1
		WHERE restaurant_id = $1 AND business_date = $2
		RETURNING next_order_number - 1
	`, identity.restaurantID, businessDate).Scan(&orderNumber); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("allocate order number: %w", err))
		return
	}

	var response createOrderResponse
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO orders (
			restaurant_id, business_date, order_number, pickup_code_hash, notes, created_by_user_id
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		RETURNING id::text, created_at
	`, identity.restaurantID, businessDate, orderNumber, pickupCodeHash[:], body.Notes, identity.userID).Scan(
		&response.ID,
		&response.CreatedAt,
	)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("insert restaurant order: %w", err))
		return
	}
	response.OrderNumber = orderNumber
	response.PickupCode = pickupCode
	response.Status = "PENDING"
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"order.created",
		"order",
		response.ID,
		map[string]any{"orderNumber": orderNumber, "status": response.Status},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		"",
		"order.created",
		map[string]any{
			"orderId":     response.ID,
			"orderNumber": orderNumber,
			"status":      response.Status,
		},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit order creation: %w", err))
		return
	}

	if err := writeSuccess(w, http.StatusCreated, "Order created.", response); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write created order response",
			"error", err,
		)
	}
}

func (controller *staffAuthController) updateOrderStatus(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	var body updateOrderStatusRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if !validOrderStatus(body.Status) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "ORDER_INVALID_STATUS",
			Message: "The order status is not supported.",
		})
		return
	}
	orderID := r.PathValue("orderID")
	if !orderIDPattern.MatchString(orderID) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusNotFound,
			Code:    "ORDER_NOT_FOUND",
			Message: "Order not found.",
		})
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin order status update: %w", err))
		return
	}
	defer tx.Rollback()

	var current string
	if err := tx.QueryRowContext(r.Context(), `
		SELECT status
		FROM orders
		WHERE id = $1 AND restaurant_id = $2
		FOR UPDATE
	`, orderID, identity.restaurantID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusNotFound,
				Code:    "ORDER_NOT_FOUND",
				Message: "Order not found.",
			})
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load restaurant order: %w", err))
		return
	}
	if !validOrderTransition(current, body.Status) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusConflict,
			Code:    "ORDER_INVALID_TRANSITION",
			Message: "The order cannot move to that status.",
		})
		return
	}

	if _, err := tx.ExecContext(r.Context(), `
		UPDATE orders SET status = $1, updated_at = now(),
			accepted_at = CASE WHEN $1 = 'ACCEPTED' THEN now() ELSE accepted_at END,
			preparing_at = CASE WHEN $1 = 'PREPARING' THEN now() ELSE preparing_at END,
			ready_at = CASE WHEN $1 = 'READY' THEN now() ELSE ready_at END,
			picked_up_at = CASE WHEN $1 = 'PICKED_UP' THEN now() ELSE picked_up_at END
		WHERE id = $2 AND restaurant_id = $3
	`, body.Status, orderID, identity.restaurantID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("update restaurant order: %w", err))
		return
	}
	ticketStatus := ticketStatusForOrder(body.Status)
	var ticketID string
	ticketErr := tx.QueryRowContext(r.Context(), `
		UPDATE queue_tickets
		SET status = $1, updated_at = now(),
			completed_at = CASE WHEN $1 IN ('PICKED_UP', 'CANCELLED') THEN now() ELSE completed_at END
		WHERE order_id = $2 AND restaurant_id = $3
		RETURNING id::text
	`, ticketStatus, orderID, identity.restaurantID).Scan(&ticketID)
	if ticketErr != nil && !errors.Is(ticketErr, sql.ErrNoRows) {
		err := ticketErr
		writeMappedError(w, r, controller.logger, fmt.Errorf("update pickup ticket status: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"order.status.updated",
		"order",
		orderID,
		map[string]string{"from": current, "to": body.Status},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		ticketID,
		"order.status.updated",
		map[string]string{"orderId": orderID, "status": body.Status, "ticketStatus": ticketStatus},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit order status update: %w", err))
		return
	}

	if err := writeSuccess(w, http.StatusOK, "Order status updated.", map[string]string{
		"status": body.Status,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write order status response",
			"error", err,
		)
	}
}

func staffIdentityFromContext(ctx context.Context) (staffIdentity, bool) {
	identity, ok := ctx.Value(staffIdentityContextKey{}).(staffIdentity)
	return identity, ok
}

func validOrderStatus(status string) bool {
	switch status {
	case "PENDING", "ACCEPTED", "PREPARING", "READY", "PICKED_UP", "CANCELLED":
		return true
	default:
		return false
	}
}

func validOrderTransition(current, next string) bool {
	switch current {
	case "PENDING":
		return next == "ACCEPTED" || next == "CANCELLED"
	case "ACCEPTED":
		return next == "PREPARING" || next == "CANCELLED"
	case "PREPARING":
		return next == "READY" || next == "CANCELLED"
	case "READY":
		return next == "PICKED_UP"
	default:
		return false
	}
}

func ticketStatusForOrder(status string) string {
	switch status {
	case "PENDING", "ACCEPTED":
		return "WAITING"
	case "PREPARING":
		return "PREPARING"
	case "READY":
		return "READY"
	case "PICKED_UP":
		return "PICKED_UP"
	case "CANCELLED":
		return "CANCELLED"
	default:
		return "WAITING"
	}
}

func randomPickupCode() (string, error) {
	code := make([]byte, 8)
	maximum := big.NewInt(int64(len(pickupCodeAlphabet)))
	for i := range code {
		index, err := rand.Int(rand.Reader, maximum)
		if err != nil {
			return "", err
		}
		code[i] = pickupCodeAlphabet[index.Int64()]
	}
	return string(code), nil
}
