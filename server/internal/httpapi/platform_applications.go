package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	businessSlugPattern      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	applicationIDPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	applicationStatusPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

type publicApplicationController struct {
	db            *sql.DB
	logger        *slog.Logger
	submitLimiter *rateLimiter
	statusLimiter *rateLimiter
}

type platformApplicationController struct {
	db     *sql.DB
	logger *slog.Logger
}

type createBusinessApplicationRequest struct {
	BusinessName       string `json:"businessName"`
	BusinessType       string `json:"businessType"`
	Slug               string `json:"slug"`
	Address            string `json:"address"`
	Phone              string `json:"phone"`
	TableQueueEnabled  bool   `json:"tableQueueEnabled"`
	PickupQueueEnabled bool   `json:"pickupQueueEnabled"`
}

type createBusinessApplicationResponse struct {
	StatusToken string `json:"statusToken"`
}

type publicApplicationStatus struct {
	BusinessName   string     `json:"businessName"`
	Status         string     `json:"status"`
	RestaurantSlug *string    `json:"restaurantSlug"`
	ReviewNote     *string    `json:"reviewNote"`
	CreatedAt      time.Time  `json:"createdAt"`
	ReviewedAt     *time.Time `json:"reviewedAt"`
}

type businessApplicationSummary struct {
	ID                 string     `json:"id"`
	BusinessName       string     `json:"businessName"`
	BusinessType       string     `json:"businessType"`
	Slug               string     `json:"slug"`
	Address            *string    `json:"address"`
	Phone              *string    `json:"phone"`
	OwnerName          string     `json:"ownerName"`
	OwnerEmail         string     `json:"ownerEmail"`
	TableQueueEnabled  bool       `json:"tableQueueEnabled"`
	PickupQueueEnabled bool       `json:"pickupQueueEnabled"`
	Status             string     `json:"status"`
	ReviewNote         *string    `json:"reviewNote"`
	CreatedAt          time.Time  `json:"createdAt"`
	ReviewedAt         *time.Time `json:"reviewedAt"`
}

type rejectBusinessApplicationRequest struct {
	ReviewNote string `json:"reviewNote"`
}

type approveBusinessApplicationResponse struct {
	BusinessName string `json:"businessName"`
	Slug         string `json:"slug"`
}

func newPublicApplicationController(db *sql.DB, logger *slog.Logger) *publicApplicationController {
	return &publicApplicationController{
		db:            db,
		logger:        logger,
		submitLimiter: newRateLimiter(8, time.Hour),
		statusLimiter: newRateLimiter(30, time.Minute),
	}
}

func newPlatformApplicationController(db *sql.DB, logger *slog.Logger) *platformApplicationController {
	return &platformApplicationController{db: db, logger: logger}
}

func registerPlatformApplicationRoutes(
	mux *http.ServeMux,
	public *publicApplicationController,
	platform *platformApplicationController,
	auth *platformAdminAuthController,
	ownerAuth *ownerAuthController,
) {
	mux.Handle(
		"POST /api/owner/applications",
		ownerAuth.requireOwner(http.HandlerFunc(public.create)),
	)
	mux.HandleFunc("GET /api/public/applications/{statusToken}", public.status)
	mux.Handle(
		"GET /api/platform/applications",
		auth.requireAdmin(http.HandlerFunc(platform.list)),
	)
	mux.Handle(
		"POST /api/platform/applications/{applicationID}/approve",
		auth.requireAdmin(http.HandlerFunc(platform.approve)),
	)
	mux.Handle(
		"POST /api/platform/applications/{applicationID}/reject",
		auth.requireAdmin(http.HandlerFunc(platform.reject)),
	)
}

func (controller *publicApplicationController) create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	owner, ok := ownerIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedOwner())
		return
	}

	var body createBusinessApplicationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if err := normalizeAndValidateApplication(&body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	ip := requestIP(r)
	if !controller.submitLimiter.allow(ip) ||
		!controller.submitLimiter.allow(ip+"\x00"+owner.email) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusTooManyRequests,
			Code:    "APPLICATION_RATE_LIMITED",
			Message: "Too many applications were submitted. Try again later.",
		})
		return
	}

	var identityConflict bool
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM restaurants WHERE lower(slug) = lower($1)
			UNION ALL
			SELECT 1 FROM restaurant_applications
			WHERE status = 'PENDING'
			  AND (lower(slug) = lower($1) OR lower(owner_email) = $2)
		)
	`, body.Slug, owner.email).Scan(&identityConflict)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("check business application uniqueness: %w", err))
		return
	}
	if identityConflict {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusConflict,
			Code:    "APPLICATION_CONFLICT",
			Message: "That business URL or owner email is already in use.",
		})
		return
	}

	statusTokenBytes := make([]byte, 32)
	if _, err := rand.Read(statusTokenBytes); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate application status token: %w", err))
		return
	}
	statusToken := base64.RawURLEncoding.EncodeToString(statusTokenBytes)
	statusTokenHash := sha256.Sum256(statusTokenBytes)
	clear(statusTokenBytes)

	_, err = controller.db.ExecContext(r.Context(), `
		INSERT INTO restaurant_applications (
			business_name, business_type, slug, address, phone, owner_user_id, owner_name,
			owner_email, table_queue_enabled, pickup_queue_enabled, status_token_hash
		)
		VALUES (
			$1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10, $11
		)
	`,
		body.BusinessName,
		body.BusinessType,
		body.Slug,
		body.Address,
		body.Phone,
		owner.id,
		owner.name,
		owner.email,
		body.TableQueueEnabled,
		body.PickupQueueEnabled,
		statusTokenHash[:],
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeMappedError(w, r, controller.logger, &APIError{
				Status:  http.StatusConflict,
				Code:    "APPLICATION_CONFLICT",
				Message: "That business URL or owner email is already in use.",
			})
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("create business application: %w", err))
		return
	}

	if err := writeSuccess(w, http.StatusCreated, "Application received.", createBusinessApplicationResponse{
		StatusToken: statusToken,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write business application response",
			"error", err,
		)
	}
}

func (controller *publicApplicationController) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	token := r.PathValue("statusToken")
	if !applicationStatusPattern.MatchString(token) ||
		!controller.statusLimiter.allow(requestIP(r)) {
		writeMappedError(w, r, controller.logger, applicationNotFound())
		return
	}
	decodedToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decodedToken) != 32 {
		clear(decodedToken)
		writeMappedError(w, r, controller.logger, applicationNotFound())
		return
	}
	tokenHash := sha256.Sum256(decodedToken)
	clear(decodedToken)

	var status publicApplicationStatus
	var slug sql.NullString
	var reviewNote sql.NullString
	var reviewedAt sql.NullTime
	err = controller.db.QueryRowContext(r.Context(), `
		SELECT business_name, status, slug, review_note, created_at, reviewed_at
		FROM restaurant_applications
		WHERE status_token_hash = $1
	`, tokenHash[:]).Scan(
		&status.BusinessName,
		&status.Status,
		&slug,
		&reviewNote,
		&status.CreatedAt,
		&reviewedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, applicationNotFound())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load public application status: %w", err))
		return
	}
	if slug.Valid {
		status.RestaurantSlug = &slug.String
	}
	if reviewNote.Valid {
		status.ReviewNote = &reviewNote.String
	}
	if reviewedAt.Valid {
		status.ReviewedAt = &reviewedAt.Time
	}
	if err := writeSuccess(w, http.StatusOK, "Application status loaded.", status); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write public application status response",
			"error", err,
		)
	}
}

func normalizeAndValidateApplication(body *createBusinessApplicationRequest) error {
	body.BusinessName = strings.TrimSpace(body.BusinessName)
	body.BusinessType = strings.ToUpper(strings.TrimSpace(body.BusinessType))
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.Address = strings.TrimSpace(body.Address)
	body.Phone = strings.TrimSpace(body.Phone)

	if !validTextLength(body.BusinessName, 1, 120) {
		return invalidApplication("Enter a business name of up to 120 characters.")
	}
	switch body.BusinessType {
	case "RESTAURANT", "BAKERY", "EATERY":
	default:
		return invalidApplication("Choose a restaurant, bakery, or eatery.")
	}
	if !businessSlugPattern.MatchString(body.Slug) {
		return invalidApplication("Use lowercase letters, numbers, and single hyphens for the business URL.")
	}
	if len(body.Address) > 500 {
		return invalidApplication("The address must be 500 characters or fewer.")
	}
	if len(body.Phone) > 50 {
		return invalidApplication("The phone number must be 50 characters or fewer.")
	}
	if !body.TableQueueEnabled && !body.PickupQueueEnabled {
		return invalidApplication("Choose at least one queue service.")
	}
	return nil
}

func validTextLength(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(value)
	return length >= minimum && length <= maximum
}

func invalidApplication(message string) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "APPLICATION_INVALID",
		Message: message,
	}
}

func applicationNotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    "APPLICATION_NOT_FOUND",
		Message: "Application not found.",
	}
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

func (controller *platformApplicationController) list(w http.ResponseWriter, r *http.Request) {
	admin, ok := platformAdminFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
		return
	}

	status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = "PENDING"
	}
	switch status {
	case "PENDING", "APPROVED", "REJECTED", "ALL":
	default:
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "APPLICATION_STATUS_INVALID",
			Message: "Choose a valid application status.",
		})
		return
	}
	metadata, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("encode application access audit: %w", err))
		return
	}
	if _, err := controller.db.ExecContext(r.Context(), `
		INSERT INTO platform_audit_logs (admin_id, action, metadata)
		VALUES ($1, 'APPLICATIONS_VIEWED', $2)
	`, admin.id, metadata); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("audit platform application access: %w", err))
		return
	}

	rows, err := controller.db.QueryContext(r.Context(), `
		SELECT id::text, business_name, business_type, slug, address, phone,
		       owner_name, owner_email, table_queue_enabled, pickup_queue_enabled,
		       status, review_note, created_at, reviewed_at
		FROM restaurant_applications
		WHERE $1 = 'ALL' OR status = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, status)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("list business applications: %w", err))
		return
	}
	defer rows.Close()

	applications := make([]businessApplicationSummary, 0)
	for rows.Next() {
		var application businessApplicationSummary
		var address, phone, reviewNote sql.NullString
		var reviewedAt sql.NullTime
		if err := rows.Scan(
			&application.ID,
			&application.BusinessName,
			&application.BusinessType,
			&application.Slug,
			&address,
			&phone,
			&application.OwnerName,
			&application.OwnerEmail,
			&application.TableQueueEnabled,
			&application.PickupQueueEnabled,
			&application.Status,
			&reviewNote,
			&application.CreatedAt,
			&reviewedAt,
		); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("scan business application: %w", err))
			return
		}
		application.Address = nullableString(address)
		application.Phone = nullableString(phone)
		application.ReviewNote = nullableString(reviewNote)
		if reviewedAt.Valid {
			application.ReviewedAt = &reviewedAt.Time
		}
		applications = append(applications, application)
	}
	if err := rows.Err(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("read business applications: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Business applications loaded.", applications); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write business applications response",
			"error", err,
		)
	}
}

func (controller *platformApplicationController) approve(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	admin, ok := platformAdminFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
		return
	}
	applicationID := r.PathValue("applicationID")
	if !applicationIDPattern.MatchString(applicationID) {
		writeMappedError(w, r, controller.logger, applicationNotFound())
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin application approval: %w", err))
		return
	}
	defer tx.Rollback()

	application, err := loadPendingApplicationForUpdate(r.Context(), tx, applicationID)
	if err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	var restaurantID string
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO restaurants (name, slug, address, phone, table_queue_enabled, pickup_queue_enabled)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6)
		RETURNING id::text
	`, application.BusinessName, application.Slug, application.Address, application.Phone,
		application.TableQueueEnabled, application.PickupQueueEnabled).Scan(&restaurantID); err != nil {
		if isUniqueViolation(err) {
			writeMappedError(w, r, controller.logger, applicationApprovalConflict())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("create approved restaurant: %w", err))
		return
	}
	ownerID := application.OwnerUserID.String
	if !application.OwnerUserID.Valid {
		if !application.OwnerPasswordHash.Valid {
			writeMappedError(w, r, controller.logger, applicationApprovalConflict())
			return
		}
		if err := tx.QueryRowContext(r.Context(), `
			INSERT INTO users (name, email, password_hash)
			VALUES ($1, $2, $3)
			RETURNING id::text
		`, application.OwnerName, application.OwnerEmail, application.OwnerPasswordHash.String).Scan(&ownerID); err != nil {
			if isUniqueViolation(err) {
				writeMappedError(w, r, controller.logger, applicationApprovalConflict())
				return
			}
			writeMappedError(w, r, controller.logger, fmt.Errorf("create approved restaurant owner: %w", err))
			return
		}
	} else {
		if err := tx.QueryRowContext(r.Context(), `
			SELECT id::text
			FROM users
			WHERE id = $1 AND is_active AND email_verified_at IS NOT NULL
			FOR UPDATE
		`, ownerID).Scan(&ownerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeMappedError(w, r, controller.logger, applicationApprovalConflict())
				return
			}
			writeMappedError(w, r, controller.logger, fmt.Errorf("load verified application owner: %w", err))
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO restaurant_memberships (restaurant_id, user_id, role)
		VALUES ($1, $2, 'OWNER')
	`, restaurantID, ownerID); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create approved restaurant membership: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE restaurant_applications
		SET status = 'APPROVED',
		    owner_password_hash = NULL,
		    owner_user_id = $2,
		    restaurant_id = $3,
		    reviewed_by = $4,
		    reviewed_at = now()
		WHERE id = $1
	`, applicationID, ownerID, restaurantID, admin.id); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("mark business application approved: %w", err))
		return
	}
	if err := auditApplicationDecision(r.Context(), tx, admin.id, applicationID, "APPLICATION_APPROVED"); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("audit business application approval: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit business application approval: %w", err))
		return
	}

	response := approveBusinessApplicationResponse{
		BusinessName: application.BusinessName,
		Slug:         application.Slug,
	}
	if err := writeSuccess(w, http.StatusOK, "Business approved and made live.", response); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write approved application response",
			"error", err,
		)
	}
}

func (controller *platformApplicationController) reject(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	admin, ok := platformAdminFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
		return
	}
	applicationID := r.PathValue("applicationID")
	if !applicationIDPattern.MatchString(applicationID) {
		writeMappedError(w, r, controller.logger, applicationNotFound())
		return
	}
	var body rejectBusinessApplicationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.ReviewNote = strings.TrimSpace(body.ReviewNote)
	if !validTextLength(body.ReviewNote, 1, 1000) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "APPLICATION_REVIEW_NOTE_INVALID",
			Message: "Add a review note of up to 1000 characters before declining.",
		})
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin application rejection: %w", err))
		return
	}
	defer tx.Rollback()
	if _, err := loadPendingApplicationForUpdate(r.Context(), tx, applicationID); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE restaurant_applications
		SET status = 'REJECTED',
		    owner_password_hash = NULL,
		    reviewed_by = $2,
		    reviewed_at = now(),
		    review_note = $3
		WHERE id = $1
	`, applicationID, admin.id, body.ReviewNote); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("mark business application rejected: %w", err))
		return
	}
	if err := auditApplicationDecision(r.Context(), tx, admin.id, applicationID, "APPLICATION_REJECTED"); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("audit business application rejection: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit business application rejection: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Business application declined.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write rejected application response",
			"error", err,
		)
	}
}

type pendingApplication struct {
	BusinessName       string
	Slug               string
	Address            string
	Phone              string
	OwnerName          string
	OwnerEmail         string
	OwnerUserID        sql.NullString
	OwnerPasswordHash  sql.NullString
	TableQueueEnabled  bool
	PickupQueueEnabled bool
}

func loadPendingApplicationForUpdate(
	ctx context.Context,
	tx *sql.Tx,
	applicationID string,
) (pendingApplication, error) {
	var application pendingApplication
	var status string
	err := tx.QueryRowContext(ctx, `
		SELECT business_name, slug, COALESCE(address, ''), COALESCE(phone, ''),
		       owner_name, owner_email, owner_user_id, owner_password_hash,
		       table_queue_enabled, pickup_queue_enabled, status
		FROM restaurant_applications
		WHERE id = $1
		FOR UPDATE
	`, applicationID).Scan(
		&application.BusinessName,
		&application.Slug,
		&application.Address,
		&application.Phone,
		&application.OwnerName,
		&application.OwnerEmail,
		&application.OwnerUserID,
		&application.OwnerPasswordHash,
		&application.TableQueueEnabled,
		&application.PickupQueueEnabled,
		&status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingApplication{}, applicationNotFound()
	}
	if err != nil {
		return pendingApplication{}, fmt.Errorf("load business application for review: %w", err)
	}
	if status != "PENDING" {
		return pendingApplication{}, &APIError{
			Status:  http.StatusConflict,
			Code:    "APPLICATION_ALREADY_REVIEWED",
			Message: "This application has already been reviewed.",
		}
	}
	return application, nil
}

func auditApplicationDecision(
	ctx context.Context,
	tx *sql.Tx,
	adminID string,
	applicationID string,
	action string,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO platform_audit_logs (admin_id, action, application_id)
		VALUES ($1, $2, $3)
	`, adminID, action, applicationID)
	return err
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func applicationApprovalConflict() *APIError {
	return &APIError{
		Status:  http.StatusConflict,
		Code:    "APPLICATION_APPROVAL_CONFLICT",
		Message: "The business URL or owner email is already in use. Resolve the conflict before approving.",
	}
}
