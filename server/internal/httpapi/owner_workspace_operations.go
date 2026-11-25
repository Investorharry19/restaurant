package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"restaurant/server/internal/staffauth"
)

type workspaceQueueTicket struct {
	ID            string     `json:"id"`
	TicketNumber  int64      `json:"ticketNumber"`
	DisplayName   string     `json:"displayName"`
	Phone         *string    `json:"phone"`
	PartySize     int        `json:"partySize"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	CalledAt      *time.Time `json:"calledAt"`
	QueuePosition int64      `json:"queuePosition"`
	AssignedTable *string    `json:"assignedTable"`
}

type updateQueueTicketRequest struct {
	Status string `json:"status"`
}

type workspaceTable struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Capacity         int        `json:"capacity"`
	IsActive         bool       `json:"isActive"`
	IsOccupied       bool       `json:"isOccupied"`
	AssignmentID     *string    `json:"assignmentId"`
	AssignedTicketID *string    `json:"assignedTicketId"`
	TicketNumber     *int64     `json:"ticketNumber"`
	PartySize        *int       `json:"partySize"`
	AssignedAt       *time.Time `json:"assignedAt"`
}

type tableRequest struct {
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
	IsActive bool   `json:"isActive"`
}

type assignTableRequest struct {
	TicketID string `json:"ticketId"`
}

type workspaceStaffMember struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

type workspaceStaffInvitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type createStaffInvitationRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type createStaffInvitationResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	InviteURL string    `json:"inviteUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type staffInvitationDetails struct {
	Email      string `json:"email"`
	Restaurant string `json:"restaurantName"`
	Role       string `json:"role"`
}

type acceptStaffInvitationRequest struct {
	Token    string `json:"token"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type workspaceAnalytics struct {
	TableTicketsToday              int64    `json:"tableTicketsToday"`
	WaitingNow                     int64    `json:"waitingNow"`
	ArrivedNow                     int64    `json:"arrivedNow"`
	AverageWaitMinutes             *float64 `json:"averageWaitMinutes"`
	AverageCallToArrivalMinutes    *float64 `json:"averageCallToArrivalMinutes"`
	AverageArrivalToSeatingMinutes *float64 `json:"averageArrivalToSeatingMinutes"`
	TablePartiesSeated             int64    `json:"tablePartiesSeated"`
	TableGuestsSeated              int64    `json:"tableGuestsSeated"`
	TableCancellations             int64    `json:"tableCancellations"`
	TableNoShows                   int64    `json:"tableNoShows"`
	OrdersToday                    int64    `json:"ordersToday"`
	OrdersPreparing                int64    `json:"ordersPreparing"`
	OrdersReady                    int64    `json:"ordersReady"`
	OrdersPickedUp                 int64    `json:"ordersPickedUp"`
	OrderCancellations             int64    `json:"orderCancellations"`
	AveragePreparationMinutes      *float64 `json:"averagePreparationMinutes"`
	AverageReadyToPickupMinutes    *float64 `json:"averageReadyToPickupMinutes"`
	ActiveQueueSize                int64    `json:"activeQueueSize"`
	CustomersServed                int64    `json:"customersServed"`
	Cancellations                  int64    `json:"cancellations"`
	NoShows                        int64    `json:"noShows"`
}

type workspaceNotification struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	SentAt    *time.Time `json:"sentAt"`
}

func (controller *ownerWorkspaceController) listTableQueue(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	businessDate, err := controller.currentBusinessDate(r.Context(), identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate table queue business date: %w", err))
		return
	}

	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT t.id::text, t.ticket_number, s.display_name, s.phone, t.party_size,
		       t.status, t.created_at, t.called_at,
		       CASE WHEN t.status = 'WAITING' THEN
		           row_number() OVER (PARTITION BY t.status ORDER BY t.created_at, t.id)
		       ELSE NULL END,
		       rt.name
		FROM queue_tickets t
		JOIN queues q ON q.id = t.queue_id AND q.restaurant_id = t.restaurant_id
		JOIN customer_sessions s ON s.id = t.customer_session_id
		LEFT JOIN table_assignments a
		  ON a.queue_ticket_id = t.id AND a.restaurant_id = t.restaurant_id
		 AND a.released_at IS NULL
		LEFT JOIN restaurant_tables rt
		  ON rt.id = a.table_id AND rt.restaurant_id = a.restaurant_id
		WHERE t.restaurant_id = $1 AND q.business_date = $2
		  AND t.type = 'TABLE'
		  AND t.status IN ('WAITING', 'CALLED', 'ARRIVED')
		  AND t.expires_at > now()
		ORDER BY CASE t.status WHEN 'ARRIVED' THEN 0 WHEN 'CALLED' THEN 1 ELSE 2 END,
		         t.created_at, t.id
	`, identity.restaurantID, businessDate)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant table queue: %w", err))
		return
	}
	defer rows.Close()

	tickets := make([]workspaceQueueTicket, 0)
	for rows.Next() {
		var ticket workspaceQueueTicket
		var phone, assignedTable sql.NullString
		var calledAt sql.NullTime
		var position sql.NullInt64
		if err := rows.Scan(
			&ticket.ID,
			&ticket.TicketNumber,
			&ticket.DisplayName,
			&phone,
			&ticket.PartySize,
			&ticket.Status,
			&ticket.CreatedAt,
			&calledAt,
			&position,
			&assignedTable,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant table queue: %w", err))
			return
		}
		if phone.Valid {
			ticket.Phone = &phone.String
		}
		if calledAt.Valid {
			ticket.CalledAt = &calledAt.Time
		}
		if position.Valid {
			ticket.QueuePosition = position.Int64
		}
		if assignedTable.Valid {
			ticket.AssignedTable = &assignedTable.String
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant table queue: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Table queue loaded.", tickets); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write table queue response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) updateTableTicket(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	var body updateQueueTicketRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if !validQueueTicketID(r.PathValue("ticketID")) ||
		(body.Status != "CALLED" && body.Status != "ARRIVED" &&
			body.Status != "NO_SHOW" && body.Status != "CANCELLED") {
		writeMappedError(w, r, controller.logger, invalidQueueTicket())
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin table queue update: %w", err))
		return
	}
	defer tx.Rollback()

	var current string
	var ticketNumber int64
	if err := tx.QueryRowContext(r.Context(), `
		SELECT status, ticket_number FROM queue_tickets
		WHERE id = $1 AND restaurant_id = $2 AND type = 'TABLE'
		FOR UPDATE
	`, r.PathValue("ticketID"), identity.restaurantID).Scan(&current, &ticketNumber); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, queueTicketNotFound())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load table queue ticket: %w", err))
		return
	}
	if !validTableQueueTransition(current, body.Status) {
		writeMappedError(w, r, controller.logger, invalidQueueTransition())
		return
	}
	_, err = tx.ExecContext(r.Context(), `
		UPDATE queue_tickets
		SET status = $1,
		    called_at = CASE WHEN $1 = 'CALLED' THEN now() ELSE called_at END,
		    arrived_at = CASE WHEN $1 = 'ARRIVED' THEN now() ELSE arrived_at END,
		    completed_at = CASE WHEN $1 IN ('NO_SHOW', 'CANCELLED') THEN now() ELSE completed_at END,
		    updated_at = now()
		WHERE id = $2 AND restaurant_id = $3
	`, body.Status, r.PathValue("ticketID"), identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("update table queue ticket: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"queue.ticket.status.updated",
		"queue_ticket",
		r.PathValue("ticketID"),
		map[string]string{"from": current, "to": body.Status},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		r.PathValue("ticketID"),
		"queue.ticket.updated",
		map[string]any{"ticketNumber": ticketNumber, "status": body.Status},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit table queue update: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Table ticket updated.", map[string]string{
		"status": body.Status,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write table queue update response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) callNextTableTicket(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	controller.claimNextTableTicket(w, r, identity, "")
}

func (controller *ownerWorkspaceController) callNextStaffTableTicket(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	queueID := r.PathValue("queueID")
	if !validQueueTicketID(queueID) {
		writeMappedError(w, r, controller.logger, invalidQueueTicket())
		return
	}
	controller.claimNextTableTicket(w, r, identity, queueID)
}

func (controller *ownerWorkspaceController) claimNextTableTicket(
	w http.ResponseWriter,
	r *http.Request,
	identity staffIdentity,
	queueID string,
) {
	businessDate, err := controller.currentBusinessDate(r.Context(), identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate table queue business date: %w", err))
		return
	}
	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin call-next operation: %w", err))
		return
	}
	defer tx.Rollback()

	var ticketID string
	var ticketNumber int64
	err = tx.QueryRowContext(r.Context(), `
		SELECT t.id::text, t.ticket_number
		FROM queue_tickets t
		JOIN queues q ON q.id = t.queue_id AND q.restaurant_id = t.restaurant_id
		WHERE t.restaurant_id = $1 AND q.business_date = $2
		  AND t.type = 'TABLE' AND t.status = 'WAITING' AND t.expires_at > now()
		  AND (NULLIF($3, '') IS NULL OR q.id = NULLIF($3, '')::uuid)
		ORDER BY t.created_at, t.id
		LIMIT 1
		FOR UPDATE OF t SKIP LOCKED
	`, identity.restaurantID, businessDate, queueID).Scan(&ticketID, &ticketNumber)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusConflict,
			Code:    "QUEUE_EMPTY",
			Message: "There are no waiting guests to call.",
		})
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("select next waiting table ticket: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE queue_tickets
		SET status = 'CALLED', called_at = now(), updated_at = now()
		WHERE id = $1 AND restaurant_id = $2 AND status = 'WAITING'
	`, ticketID, identity.restaurantID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("call next table ticket: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"queue.ticket.called",
		"queue_ticket",
		ticketID,
		map[string]int64{"ticketNumber": ticketNumber},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		ticketID,
		"queue.ticket.updated",
		map[string]any{"ticketNumber": ticketNumber, "status": "CALLED"},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit call-next operation: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Next guest called.", map[string]any{
		"ticketId":     ticketID,
		"ticketNumber": ticketNumber,
		"status":       "CALLED",
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write call-next response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) listTables(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT t.id::text, t.name, t.capacity, t.is_active,
		       a.id::text,
		       a.queue_ticket_id::text, q.ticket_number, q.party_size, a.assigned_at
		FROM restaurant_tables t
		LEFT JOIN table_assignments a
		  ON a.table_id = t.id AND a.restaurant_id = t.restaurant_id AND a.released_at IS NULL
		LEFT JOIN queue_tickets q
		  ON q.id = a.queue_ticket_id AND q.restaurant_id = a.restaurant_id
		WHERE t.restaurant_id = $1
		ORDER BY t.name, t.id
	`, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant tables: %w", err))
		return
	}
	defer rows.Close()

	tables := make([]workspaceTable, 0)
	for rows.Next() {
		var table workspaceTable
		var assignmentID, ticketID sql.NullString
		var ticketNumber, partySize sql.NullInt64
		var assignedAt sql.NullTime
		if err := rows.Scan(
			&table.ID,
			&table.Name,
			&table.Capacity,
			&table.IsActive,
			&assignmentID,
			&ticketID,
			&ticketNumber,
			&partySize,
			&assignedAt,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant table: %w", err))
			return
		}
		table.IsOccupied = ticketID.Valid
		if assignmentID.Valid {
			table.AssignmentID = &assignmentID.String
		}
		if ticketID.Valid {
			table.AssignedTicketID = &ticketID.String
		}
		if ticketNumber.Valid {
			number := ticketNumber.Int64
			table.TicketNumber = &number
		}
		if partySize.Valid {
			size := int(partySize.Int64)
			table.PartySize = &size
		}
		if assignedAt.Valid {
			table.AssignedAt = &assignedAt.Time
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant tables: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Tables loaded.", tables); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write restaurant tables response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) createTable(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	var body tableRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if !validTableInput(body) {
		writeMappedError(w, r, controller.logger, invalidTable())
		return
	}

	var table workspaceTable
	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin restaurant table creation: %w", err))
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO restaurant_tables (restaurant_id, name, capacity, is_active)
		VALUES ($1, $2, $3, true)
		RETURNING id::text, name, capacity, is_active
	`, identity.restaurantID, body.Name, body.Capacity).Scan(
		&table.ID, &table.Name, &table.Capacity, &table.IsActive,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeMappedError(w, r, controller.logger, tableConflict())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("create restaurant table: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"table.created",
		"restaurant_table",
		table.ID,
		map[string]any{"name": table.Name, "capacity": table.Capacity},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		"",
		"table.updated",
		map[string]any{"tableId": table.ID, "name": table.Name, "capacity": table.Capacity, "isActive": table.IsActive},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit restaurant table creation: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusCreated, "Table created.", table); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write created table response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) updateTable(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	tableID := r.PathValue("tableID")
	if !validQueueTicketID(tableID) {
		writeMappedError(w, r, controller.logger, invalidTable())
		return
	}
	var body tableRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if !validTableInput(body) {
		writeMappedError(w, r, controller.logger, invalidTable())
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin restaurant table update: %w", err))
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `
		UPDATE restaurant_tables t
		SET name = $1, capacity = $2, is_active = $3, updated_at = now()
		WHERE t.id = $4 AND t.restaurant_id = $5
		  AND ($3 OR NOT EXISTS (
		      SELECT 1 FROM table_assignments a
		      WHERE a.table_id = t.id AND a.released_at IS NULL
		  ))
	`, body.Name, body.Capacity, body.IsActive, tableID, identity.restaurantID)
	if err != nil {
		if isUniqueViolation(err) {
			writeMappedError(w, r, controller.logger, tableConflict())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("update restaurant table: %w", err))
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("check restaurant table update: %w", err))
		return
	}
	if rowsAffected == 0 {
		writeMappedError(w, r, controller.logger, tableNotFoundOrOccupied())
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"table.updated",
		"restaurant_table",
		tableID,
		map[string]any{"name": body.Name, "capacity": body.Capacity, "isActive": body.IsActive},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		"",
		"table.updated",
		map[string]any{"tableId": tableID, "name": body.Name, "capacity": body.Capacity, "isActive": body.IsActive},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit restaurant table update: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Table updated.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write table update response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) assignTable(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	tableID := r.PathValue("tableID")
	if !validQueueTicketID(tableID) {
		writeMappedError(w, r, controller.logger, invalidTable())
		return
	}
	var body assignTableRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if !validQueueTicketID(body.TicketID) {
		writeMappedError(w, r, controller.logger, invalidQueueTicket())
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin table assignment: %w", err))
		return
	}
	defer tx.Rollback()

	var tableName string
	var capacity int
	var active bool
	if err := tx.QueryRowContext(r.Context(), `
		SELECT name, capacity, is_active FROM restaurant_tables
		WHERE id = $1 AND restaurant_id = $2 FOR UPDATE
	`, tableID, identity.restaurantID).Scan(&tableName, &capacity, &active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, tableNotFoundOrOccupied())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load table for assignment: %w", err))
		return
	}
	if !active {
		writeMappedError(w, r, controller.logger, tableNotFoundOrOccupied())
		return
	}

	var partySize int
	var ticketNumber int64
	var status string
	if err := tx.QueryRowContext(r.Context(), `
		SELECT party_size, status, ticket_number FROM queue_tickets
		WHERE id = $1 AND restaurant_id = $2 AND type = 'TABLE'
		FOR UPDATE
	`, body.TicketID, identity.restaurantID).Scan(&partySize, &status, &ticketNumber); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, queueTicketNotFound())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load ticket for table assignment: %w", err))
		return
	}
	if status != "ARRIVED" || partySize > capacity {
		writeMappedError(w, r, controller.logger, assignmentNotAllowed())
		return
	}

	var assignmentID string
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO table_assignments (
			restaurant_id, table_id, queue_ticket_id, assigned_by_user_id
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, identity.restaurantID, tableID, body.TicketID, identity.userID).Scan(&assignmentID)
	if err != nil {
		if isUniqueViolation(err) {
			writeMappedError(w, r, controller.logger, tableAssignmentConflict())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("create table assignment: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE queue_tickets
		SET status = 'SEATED', completed_at = now(), updated_at = now()
		WHERE id = $1 AND restaurant_id = $2
	`, body.TicketID, identity.restaurantID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("seat table queue ticket: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"table.assignment.created",
		"table_assignment",
		assignmentID,
		map[string]any{"tableId": tableID, "ticketId": body.TicketID},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		body.TicketID,
		"table.assigned",
		map[string]any{
			"ticketNumber": ticketNumber,
			"tableName":    tableName,
			"status":       "SEATED",
		},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit table assignment: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusCreated, "Guest seated.", map[string]string{
		"assignmentId": assignmentID,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write table assignment response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) releaseTable(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	tableID, assignmentID := r.PathValue("tableID"), r.PathValue("assignmentID")
	if !validQueueTicketID(tableID) || !validQueueTicketID(assignmentID) {
		writeMappedError(w, r, controller.logger, tableNotFoundOrOccupied())
		return
	}
	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin table release: %w", err))
		return
	}
	defer tx.Rollback()
	var ticketID string
	err = tx.QueryRowContext(r.Context(), `
		UPDATE table_assignments
		SET released_at = now()
		WHERE id = $1 AND table_id = $2 AND restaurant_id = $3 AND released_at IS NULL
		RETURNING queue_ticket_id::text
	`, assignmentID, tableID, identity.restaurantID).Scan(&ticketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, tableNotFoundOrOccupied())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("release restaurant table: %w", err))
		return
	}
	var tableName string
	if err := tx.QueryRowContext(r.Context(), `
		SELECT name FROM restaurant_tables
		WHERE id = $1 AND restaurant_id = $2
	`, tableID, identity.restaurantID).Scan(&tableName); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load released table: %w", err))
		return
	}
	if err := recordRestaurantAudit(
		r.Context(),
		tx,
		identity,
		"table.assignment.released",
		"table_assignment",
		assignmentID,
		map[string]string{"tableId": tableID, "ticketId": ticketID},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := enqueueRestaurantEvent(
		r.Context(),
		tx,
		identity.restaurantID,
		ticketID,
		"table.released",
		map[string]string{"tableName": tableName},
	); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit table release: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Table released.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write table release response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) getAnalytics(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	businessDate, err := controller.currentBusinessDate(r.Context(), identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate analytics business date: %w", err))
		return
	}
	var result workspaceAnalytics
	var averageWait, averageCallToArrival, averageArrivalToSeating sql.NullFloat64
	var tablePartiesSeated, tableGuestsSeated, activeTableQueue int64
	if err := controller.db.QueryRowContext(r.Context(), `
		SELECT count(*) FILTER (WHERE t.type = 'TABLE'),
		       count(*) FILTER (
		           WHERE t.type = 'TABLE' AND t.status = 'WAITING' AND t.expires_at > now()
		       ),
		       count(*) FILTER (
		           WHERE t.type = 'TABLE' AND t.status = 'ARRIVED' AND t.expires_at > now()
		       ),
		       avg(EXTRACT(EPOCH FROM (t.called_at - t.created_at)) / 60)
		           FILTER (
		               WHERE t.called_at IS NOT NULL
		                 AND t.called_at >= t.created_at
		           ),
		       avg(EXTRACT(EPOCH FROM (t.arrived_at - t.called_at)) / 60)
		           FILTER (
		               WHERE t.arrived_at IS NOT NULL AND t.called_at IS NOT NULL
		                 AND t.arrived_at >= t.called_at
		           ),
		       avg(EXTRACT(EPOCH FROM (assignment.assigned_at - t.arrived_at)) / 60)
		           FILTER (
		               WHERE t.status = 'SEATED' AND assignment.assigned_at IS NOT NULL
		                 AND t.arrived_at IS NOT NULL
		                 AND assignment.assigned_at >= t.arrived_at
		           ),
		       count(*) FILTER (WHERE t.type = 'TABLE' AND t.status = 'SEATED'),
		       COALESCE(sum(t.party_size) FILTER (
		           WHERE t.type = 'TABLE' AND t.status = 'SEATED'
		       ), 0),
		       count(*) FILTER (WHERE t.type = 'TABLE' AND t.status = 'CANCELLED'),
		       count(*) FILTER (WHERE t.type = 'TABLE' AND t.status = 'NO_SHOW'),
		       count(*) FILTER (
		           WHERE t.type = 'TABLE'
		             AND t.status IN ('WAITING', 'CALLED', 'ARRIVED')
		             AND t.expires_at > now()
		       )
		FROM queue_tickets t
		JOIN queues q ON q.id = t.queue_id AND q.restaurant_id = t.restaurant_id
		LEFT JOIN LATERAL (
		    SELECT min(a.assigned_at) AS assigned_at
		    FROM table_assignments a
		    WHERE a.queue_ticket_id = t.id AND a.restaurant_id = t.restaurant_id
		) assignment ON true
		WHERE t.restaurant_id = $1 AND q.business_date = $2
	`, identity.restaurantID, businessDate).Scan(
		&result.TableTicketsToday,
		&result.WaitingNow,
		&result.ArrivedNow,
		&averageWait,
		&averageCallToArrival,
		&averageArrivalToSeating,
		&tablePartiesSeated,
		&tableGuestsSeated,
		&result.TableCancellations,
		&result.TableNoShows,
		&activeTableQueue,
	); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load table analytics: %w", err))
		return
	}
	if averageWait.Valid {
		result.AverageWaitMinutes = &averageWait.Float64
	}
	if averageCallToArrival.Valid {
		result.AverageCallToArrivalMinutes = &averageCallToArrival.Float64
	}
	if averageArrivalToSeating.Valid {
		result.AverageArrivalToSeatingMinutes = &averageArrivalToSeating.Float64
	}
	result.TablePartiesSeated = tablePartiesSeated
	result.TableGuestsSeated = tableGuestsSeated
	var averagePreparation, averageReadyToPickup sql.NullFloat64
	var activePickupOrders int64
	if err := controller.db.QueryRowContext(r.Context(), `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'PREPARING'),
		       count(*) FILTER (WHERE status = 'READY'),
		       count(*) FILTER (WHERE status = 'PICKED_UP'),
		       count(*) FILTER (WHERE status = 'CANCELLED'),
		       avg(EXTRACT(EPOCH FROM (ready_at - preparing_at)) / 60)
		           FILTER (
		               WHERE preparing_at IS NOT NULL AND ready_at IS NOT NULL
		                 AND ready_at >= preparing_at
		           ),
		       avg(EXTRACT(EPOCH FROM (picked_up_at - ready_at)) / 60)
		           FILTER (
		               WHERE ready_at IS NOT NULL AND picked_up_at IS NOT NULL
		                 AND picked_up_at >= ready_at
		           ),
		       count(*) FILTER (WHERE status IN ('PENDING', 'ACCEPTED', 'PREPARING', 'READY'))
		FROM orders
		WHERE restaurant_id = $1 AND business_date = $2
	`, identity.restaurantID, businessDate).Scan(
		&result.OrdersToday,
		&result.OrdersPreparing,
		&result.OrdersReady,
		&result.OrdersPickedUp,
		&result.OrderCancellations,
		&averagePreparation,
		&averageReadyToPickup,
		&activePickupOrders,
	); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load order analytics: %w", err))
		return
	}
	if averagePreparation.Valid {
		result.AveragePreparationMinutes = &averagePreparation.Float64
	}
	if averageReadyToPickup.Valid {
		result.AverageReadyToPickupMinutes = &averageReadyToPickup.Float64
	}
	result.ActiveQueueSize = activeTableQueue + activePickupOrders
	result.CustomersServed = tableGuestsSeated + result.OrdersPickedUp
	result.Cancellations = result.TableCancellations + result.OrderCancellations
	result.NoShows = result.TableNoShows
	if err := writeSuccess(w, http.StatusOK, "Restaurant analytics loaded.", result); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write restaurant analytics response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) listNotifications(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT id::text, type, title, body, status, created_at, sent_at
		FROM notifications
		WHERE restaurant_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant notifications: %w", err))
		return
	}
	defer rows.Close()
	notifications := make([]workspaceNotification, 0)
	for rows.Next() {
		var notification workspaceNotification
		var sentAt sql.NullTime
		if err := rows.Scan(
			&notification.ID, &notification.Type, &notification.Title,
			&notification.Body, &notification.Status, &notification.CreatedAt, &sentAt,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant notification: %w", err))
			return
		}
		if sentAt.Valid {
			notification.SentAt = &sentAt.Time
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant notifications: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Notification history loaded.", notifications); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write notification history response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) listWorkspaceStaff(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT u.id::text, u.name, u.email, m.role
		FROM restaurant_memberships m
		JOIN users u ON u.id = m.user_id AND u.is_active
		WHERE m.restaurant_id = $1 AND m.is_active
		ORDER BY CASE m.role WHEN 'OWNER' THEN 0 WHEN 'MANAGER' THEN 1 ELSE 2 END, u.name
	`, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant staff: %w", err))
		return
	}
	members := make([]workspaceStaffMember, 0)
	for rows.Next() {
		var member workspaceStaffMember
		if err := rows.Scan(&member.UserID, &member.Name, &member.Email, &member.Role); err != nil {
			rows.Close()
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant staff: %w", err))
			return
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant staff: %w", err))
		return
	}
	rows.Close()

	inviteRows, err := controller.db.QueryContext(r.Context(), `
		SELECT id::text, email, role, created_at, expires_at
		FROM restaurant_staff_invitations
		WHERE restaurant_id = $1 AND accepted_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 100
	`, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list restaurant staff invitations: %w", err))
		return
	}
	defer inviteRows.Close()
	invitations := make([]workspaceStaffInvitation, 0)
	for inviteRows.Next() {
		var invitation workspaceStaffInvitation
		if err := inviteRows.Scan(
			&invitation.ID,
			&invitation.Email,
			&invitation.Role,
			&invitation.CreatedAt,
			&invitation.ExpiresAt,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan restaurant staff invitation: %w", err))
			return
		}
		invitations = append(invitations, invitation)
	}
	if err := inviteRows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read restaurant staff invitations: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Restaurant staff loaded.", map[string]any{
		"members":     members,
		"invitations": invitations,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write restaurant staff response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) createStaffInvitation(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	sender, ok := controller.ownerAuth.emailSender.(StaffInvitationEmailSender)
	if !ok {
		writeMappedError(w, r, controller.logger, emailDeliveryUnavailable())
		return
	}
	var body createStaffInvitationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	body.Role = strings.TrimSpace(body.Role)
	if !validEmailAddress(body.Email) || (body.Role != "MANAGER" && body.Role != "STAFF") {
		writeMappedError(w, r, controller.logger, invalidStaffInvitation())
		return
	}
	if !controller.inviteLimiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate staff invitation token: %w", err))
		return
	}
	tokenString := base64.RawURLEncoding.EncodeToString(token)
	tokenHash := sha256.Sum256(token)
	clear(token)
	expiresAt := time.Now().UTC().Add(72 * time.Hour)

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin staff invitation: %w", err))
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE restaurant_staff_invitations
		SET accepted_at = now()
		WHERE restaurant_id = $1 AND lower(email) = $2
		  AND accepted_at IS NULL
	`, identity.restaurantID, body.Email); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("replace pending staff invitation: %w", err))
		return
	}
	var result createStaffInvitationResponse
	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO restaurant_staff_invitations (
			restaurant_id, email, role, token_hash, invited_by_user_id, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, email, role, expires_at
	`, identity.restaurantID, body.Email, body.Role, tokenHash[:], identity.userID, expiresAt).Scan(
		&result.ID, &result.Email, &result.Role, &result.ExpiresAt,
	)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create restaurant staff invitation: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit staff invitation: %w", err))
		return
	}

	inviteURL, err := controller.staffInvitationURL(tokenString)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("build staff invitation URL: %w", err))
		return
	}
	result.InviteURL = inviteURL
	if err := sender.SendStaffInvitationEmail(
		r.Context(), result.Email, inviteURL, identity.restaurantName, result.Role,
	); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("send staff invitation email: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusCreated, "Staff invitation sent.", result); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write staff invitation response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) getStaffInvitation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !controller.inviteLimiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}
	tokenHash, err := staffInvitationTokenHash(r.PathValue("token"))
	if err != nil {
		writeMappedError(w, r, controller.logger, invalidStaffInvitationToken())
		return
	}
	var details staffInvitationDetails
	err = controller.db.QueryRowContext(r.Context(), `
		SELECT i.email, r.name, i.role
		FROM restaurant_staff_invitations i
		JOIN restaurants r ON r.id = i.restaurant_id AND r.is_active
		WHERE i.token_hash = $1 AND i.accepted_at IS NULL AND i.expires_at > now()
	`, tokenHash[:]).Scan(&details.Email, &details.Restaurant, &details.Role)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, invalidStaffInvitationToken())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load staff invitation: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Staff invitation loaded.", details); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write staff invitation response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) acceptStaffInvitation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	if !controller.inviteLimiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}
	var body acceptStaffInvitationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	tokenHash, err := staffInvitationTokenHash(body.Token)
	if err != nil || len(body.Password) < 12 || len(body.Password) > 1024 ||
		strings.TrimSpace(body.Password) == "" {
		writeMappedError(w, r, controller.logger, invalidStaffInvitationAcceptance())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) > 120 {
		writeMappedError(w, r, controller.logger, invalidStaffInvitationAcceptance())
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin staff invitation acceptance: %w", err))
		return
	}
	defer tx.Rollback()

	var invitationID, restaurantID, restaurantName, restaurantSlug, email, role string
	err = tx.QueryRowContext(r.Context(), `
		SELECT i.id::text, r.id::text, r.name, r.slug, i.email, i.role
		FROM restaurant_staff_invitations i
		JOIN restaurants r ON r.id = i.restaurant_id AND r.is_active
		WHERE i.token_hash = $1 AND i.accepted_at IS NULL AND i.expires_at > now()
		FOR UPDATE OF i
	`, tokenHash[:]).Scan(
		&invitationID, &restaurantID, &restaurantName, &restaurantSlug, &email, &role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, invalidStaffInvitationToken())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("lock staff invitation: %w", err))
		return
	}

	var identity staffIdentity
	identity.restaurantID = restaurantID
	identity.restaurantName = restaurantName
	identity.restaurantSlug = restaurantSlug
	identity.role = role
	var passwordHash string
	err = tx.QueryRowContext(r.Context(), `
		SELECT id::text, name, password_hash
		FROM users
		WHERE lower(email) = $1 AND is_active
		FOR UPDATE
	`, email).Scan(&identity.userID, &identity.userName, &passwordHash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if !validTextLength(body.Name, 1, 120) {
			writeMappedError(w, r, controller.logger, invalidStaffInvitationAcceptance())
			return
		}
		passwordHash, err = staffauth.HashPassword([]byte(body.Password))
		if err != nil {
			writeMappedError(w, r, controller.logger, invalidStaffInvitationAcceptance())
			return
		}
		err = tx.QueryRowContext(r.Context(), `
			INSERT INTO users (name, email, password_hash, email_verified_at)
			VALUES ($1, $2, $3, now())
			RETURNING id::text, name
		`, body.Name, email, passwordHash).Scan(&identity.userID, &identity.userName)
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("create invited staff account: %w", err))
			return
		}
	case err != nil:
		writeMappedError(w, r, controller.logger, fmt.Errorf("load invited staff account: %w", err))
		return
	case !staffauth.VerifyPassword(passwordHash, []byte(body.Password)):
		writeMappedError(w, r, controller.logger, invalidStaffInvitationAcceptance())
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE users SET email_verified_at = COALESCE(email_verified_at, now())
		WHERE id = $1
	`, identity.userID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("verify invited staff account: %w", err))
		return
	}

	var existingRole string
	err = tx.QueryRowContext(r.Context(), `
		SELECT role FROM restaurant_memberships
		WHERE restaurant_id = $1 AND user_id = $2
		FOR UPDATE
	`, restaurantID, identity.userID).Scan(&existingRole)
	switch {
	case err == nil:
		writeMappedError(w, r, controller.logger, existingWorkspaceMembership())
		return
	case !errors.Is(err, sql.ErrNoRows):
		writeMappedError(w, r, controller.logger, fmt.Errorf("check invited staff membership: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO restaurant_memberships (restaurant_id, user_id, role)
		VALUES ($1, $2, $3)
	`, restaurantID, identity.userID, role); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("add invited restaurant staff member: %w", err))
		return
	}

	sessionToken := make([]byte, 32)
	if _, err := rand.Read(sessionToken); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate invited staff session: %w", err))
		return
	}
	sessionTokenString := base64.RawURLEncoding.EncodeToString(sessionToken)
	sessionTokenHash := sha256.Sum256(sessionToken)
	clear(sessionToken)
	expiresAt := time.Now().Add(controller.staffAuth.sessionTTL)
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE restaurant_staff_invitations SET accepted_at = now() WHERE id = $1
	`, invitationID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("consume staff invitation: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO staff_sessions (user_id, session_token_hash, active_restaurant_id, expires_at)
		VALUES ($1, $2, $3, $4)
	`, identity.userID, sessionTokenHash[:], restaurantID, expiresAt); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create invited staff session: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit staff invitation acceptance: %w", err))
		return
	}

	controller.staffAuth.setSessionCookie(w, sessionTokenString, expiresAt)
	if err := writeSuccess(w, http.StatusOK, "Staff invitation accepted.", authResponse(identity)); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write staff invitation acceptance response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) deactivateWorkspaceStaff(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	userID := r.PathValue("userID")
	if !validQueueTicketID(userID) || userID == identity.userID {
		writeMappedError(w, r, controller.logger, invalidWorkspaceMember())
		return
	}
	result, err := controller.db.ExecContext(r.Context(), `
		UPDATE restaurant_memberships
		SET is_active = false
		WHERE restaurant_id = $1 AND user_id = $2 AND is_active
		  AND role IN ('MANAGER', 'STAFF')
		  AND ($3 = 'OWNER' OR role = 'STAFF')
	`, identity.restaurantID, userID, identity.role)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("deactivate restaurant staff member: %w", err))
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("check staff deactivation: %w", err))
		return
	}
	if rowsAffected == 0 {
		writeMappedError(w, r, controller.logger, invalidWorkspaceMember())
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Staff access removed.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write staff deactivation response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) revokeStaffInvitation(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	invitationID := r.PathValue("invitationID")
	if !validQueueTicketID(invitationID) {
		writeMappedError(w, r, controller.logger, invalidStaffInvitation())
		return
	}
	result, err := controller.db.ExecContext(r.Context(), `
		UPDATE restaurant_staff_invitations
		SET accepted_at = now()
		WHERE id = $1 AND restaurant_id = $2 AND accepted_at IS NULL
	`, invitationID, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("revoke restaurant staff invitation: %w", err))
		return
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("check invitation revocation: %w", err))
		return
	}
	if rowsAffected == 0 {
		writeMappedError(w, r, controller.logger, invalidStaffInvitation())
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Staff invitation revoked.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write staff invitation revocation response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) currentBusinessDate(ctx context.Context, restaurantID string) (string, error) {
	var timezone string
	if err := controller.db.QueryRowContext(ctx, `
		SELECT timezone FROM restaurants WHERE id = $1 AND is_active
	`, restaurantID).Scan(&timezone); err != nil {
		return "", err
	}
	return restaurantBusinessDate(time.Now(), timezone)
}

func (controller *ownerWorkspaceController) staffInvitationURL(token string) (string, error) {
	base, err := url.Parse(controller.appBaseURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("application base URL must be absolute")
	}
	base.Path = "/staff/accept-invite"
	query := base.Query()
	query.Set("token", token)
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func staffInvitationTokenHash(token string) ([32]byte, error) {
	var empty [32]byte
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		clear(raw)
		return empty, fmt.Errorf("invalid staff invitation token")
	}
	hash := sha256.Sum256(raw)
	clear(raw)
	return hash, nil
}

func validTableQueueTransition(current, next string) bool {
	switch current {
	case "WAITING":
		return next == "CALLED" || next == "CANCELLED"
	case "CALLED":
		return next == "ARRIVED" || next == "NO_SHOW" || next == "CANCELLED"
	case "ARRIVED":
		return next == "NO_SHOW" || next == "CANCELLED"
	default:
		return false
	}
}

func validTableInput(table tableRequest) bool {
	return validTextLength(table.Name, 1, 80) && table.Capacity >= 1 && table.Capacity <= 30
}

func validQueueTicketID(value string) bool {
	return orderIDPattern.MatchString(value)
}

func invalidQueueTicket() *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "QUEUE_TICKET_INVALID", Message: "Choose a valid table ticket."}
}

func queueTicketNotFound() *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "QUEUE_TICKET_NOT_FOUND", Message: "Table ticket not found."}
}

func invalidQueueTransition() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "QUEUE_TICKET_INVALID_TRANSITION", Message: "This ticket cannot move to that status."}
}

func invalidTable() *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "TABLE_INVALID", Message: "Enter a table name and capacity from 1 to 30."}
}

func tableConflict() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "TABLE_NAME_EXISTS", Message: "A table with that name already exists."}
}

func tableNotFoundOrOccupied() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "TABLE_UNAVAILABLE", Message: "That table is unavailable or currently assigned."}
}

func assignmentNotAllowed() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "TABLE_ASSIGNMENT_INVALID", Message: "The guest must be marked arrived and fit the table capacity before assignment."}
}

func tableAssignmentConflict() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "TABLE_ASSIGNMENT_CONFLICT", Message: "The table or ticket is already assigned."}
}

func invalidStaffInvitation() *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "STAFF_INVITATION_INVALID", Message: "Enter a valid email address and staff role."}
}

func invalidStaffInvitationToken() *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "STAFF_INVITATION_UNAVAILABLE", Message: "This invitation link is invalid or has expired."}
}

func invalidStaffInvitationAcceptance() *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "STAFF_INVITATION_ACCEPTANCE_INVALID", Message: "Enter your name and a password of at least 12 characters, then try again."}
}

func existingWorkspaceMembership() *APIError {
	return &APIError{Status: http.StatusConflict, Code: "STAFF_ALREADY_MEMBER", Message: "This account already has access to the restaurant."}
}

func invalidWorkspaceMember() *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "STAFF_MEMBER_NOT_FOUND", Message: "The staff member is unavailable or cannot be removed."}
}

func emailDeliveryUnavailable() *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "EMAIL_DELIVERY_UNAVAILABLE", Message: "Staff invitation email is not configured. Try again later."}
}
