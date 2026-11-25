package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"restaurant/server/internal/realtime"
)

type ownerWorkspaceController struct {
	db            *sql.DB
	logger        *slog.Logger
	ownerAuth     *ownerAuthController
	staffAuth     *staffAuthController
	appBaseURL    string
	inviteLimiter *rateLimiter
	eventHub      *realtime.Hub
}

type workspaceOverview struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Description        *string `json:"description"`
	Address            *string `json:"address"`
	Phone              *string `json:"phone"`
	Timezone           string  `json:"timezone"`
	Role               string  `json:"role"`
	GuestURL           string  `json:"guestUrl"`
	TableQueueEnabled  bool    `json:"tableQueueEnabled"`
	PickupQueueEnabled bool    `json:"pickupQueueEnabled"`
	WaitingTables      int64   `json:"waitingTables"`
	ArrivedTables      int64   `json:"arrivedTables"`
	OpenOrders         int64   `json:"openOrders"`
	AvailableTables    int64   `json:"availableTables"`
	ActiveStaff        int64   `json:"activeStaff"`
}

type workspaceSettingsRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	Address            string `json:"address"`
	Phone              string `json:"phone"`
	TableQueueEnabled  bool   `json:"tableQueueEnabled"`
	PickupQueueEnabled bool   `json:"pickupQueueEnabled"`
}

func newOwnerWorkspaceController(
	db *sql.DB,
	logger *slog.Logger,
	ownerAuth *ownerAuthController,
	staffAuth *staffAuthController,
	appBaseURL string,
	eventHub *realtime.Hub,
) *ownerWorkspaceController {
	return &ownerWorkspaceController{
		db:            db,
		logger:        logger,
		ownerAuth:     ownerAuth,
		staffAuth:     staffAuth,
		appBaseURL:    appBaseURL,
		inviteLimiter: newRateLimiter(12, 15*time.Minute),
		eventHub:      eventHub,
	}
}

func registerOwnerWorkspaceRoutes(
	mux *http.ServeMux,
	controller *ownerWorkspaceController,
) {
	mux.Handle("GET /api/owner/workspaces/{slug}",
		controller.requireMember(http.HandlerFunc(controller.getOverview)))
	mux.Handle("PATCH /api/owner/workspaces/{slug}",
		controller.requireManager(http.HandlerFunc(controller.updateSettings)))
	mux.Handle("GET /api/owner/workspaces/{slug}/qr",
		controller.requireMember(http.HandlerFunc(controller.getQR)))
	mux.Handle("GET /api/owner/workspaces/{slug}/queue",
		controller.requireMember(http.HandlerFunc(controller.listTableQueue)))
	mux.Handle("PATCH /api/owner/workspaces/{slug}/queue/{ticketID}",
		controller.requireMember(http.HandlerFunc(controller.updateTableTicket)))
	mux.Handle("POST /api/owner/workspaces/{slug}/queue/call-next",
		controller.requireMember(http.HandlerFunc(controller.callNextTableTicket)))
	mux.Handle("GET /api/owner/workspaces/{slug}/events",
		controller.requireMember(http.HandlerFunc(controller.workspaceEvents)))
	mux.Handle("GET /api/staff/events",
		controller.staffAuth.requireStaff(http.HandlerFunc(controller.staffEvents)))
	mux.Handle("POST /api/staff/queues/{queueID}/call-next",
		controller.staffAuth.requireStaff(http.HandlerFunc(controller.callNextStaffTableTicket)))
	mux.Handle("GET /api/owner/workspaces/{slug}/tables",
		controller.requireMember(http.HandlerFunc(controller.listTables)))
	mux.Handle("POST /api/owner/workspaces/{slug}/tables",
		controller.requireManager(http.HandlerFunc(controller.createTable)))
	mux.Handle("PATCH /api/owner/workspaces/{slug}/tables/{tableID}",
		controller.requireManager(http.HandlerFunc(controller.updateTable)))
	mux.Handle("POST /api/owner/workspaces/{slug}/tables/{tableID}/assignments",
		controller.requireMember(http.HandlerFunc(controller.assignTable)))
	mux.Handle("DELETE /api/owner/workspaces/{slug}/tables/{tableID}/assignments/{assignmentID}",
		controller.requireMember(http.HandlerFunc(controller.releaseTable)))
	mux.Handle("GET /api/owner/workspaces/{slug}/orders",
		controller.requireMember(http.HandlerFunc(controller.staffAuth.listOrders)))
	mux.Handle("POST /api/owner/workspaces/{slug}/orders",
		controller.requireMember(http.HandlerFunc(controller.staffAuth.createOrder)))
	mux.Handle("PATCH /api/owner/workspaces/{slug}/orders/{orderID}/status",
		controller.requireMember(http.HandlerFunc(controller.staffAuth.updateOrderStatus)))
	mux.Handle("GET /api/owner/workspaces/{slug}/staff",
		controller.requireManager(http.HandlerFunc(controller.listWorkspaceStaff)))
	mux.Handle("POST /api/owner/workspaces/{slug}/staff/invitations",
		controller.requireManager(http.HandlerFunc(controller.createStaffInvitation)))
	mux.Handle("DELETE /api/owner/workspaces/{slug}/staff/{userID}",
		controller.requireManager(http.HandlerFunc(controller.deactivateWorkspaceStaff)))
	mux.Handle("DELETE /api/owner/workspaces/{slug}/staff/invitations/{invitationID}",
		controller.requireManager(http.HandlerFunc(controller.revokeStaffInvitation)))
	mux.Handle("GET /api/owner/workspaces/{slug}/analytics",
		controller.requireMember(http.HandlerFunc(controller.getAnalytics)))
	mux.Handle("GET /api/owner/workspaces/{slug}/notifications",
		controller.requireMember(http.HandlerFunc(controller.listNotifications)))
	mux.HandleFunc("GET /api/public/staff-invitations/{token}", controller.getStaffInvitation)
	mux.HandleFunc("POST /api/public/staff-invitations/accept", controller.acceptStaffInvitation)
}

func (controller *ownerWorkspaceController) requireMember(next http.Handler) http.Handler {
	return controller.requireWorkspaceRole(next, "OWNER", "MANAGER", "STAFF")
}

func (controller *ownerWorkspaceController) requireManager(next http.Handler) http.Handler {
	return controller.requireWorkspaceRole(next, "OWNER", "MANAGER")
}

func (controller *ownerWorkspaceController) requireWorkspaceRole(
	next http.Handler,
	allowedRoles ...string,
) http.Handler {
	resolveWorkspace := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var identity staffIdentity
		if staffIdentity, ok := staffIdentityFromContext(r.Context()); ok {
			identity = staffIdentity
		} else if ownerIdentity, ok := ownerIdentityFromContext(r.Context()); ok {
			identity.userID = ownerIdentity.id
			identity.userName = ownerIdentity.name
			err := controller.db.QueryRowContext(r.Context(), `
				SELECT r.id::text, r.name, r.slug, m.role
				FROM restaurant_memberships m
				JOIN restaurants r ON r.id = m.restaurant_id AND r.is_active
				WHERE m.user_id = $1 AND m.is_active AND lower(r.slug) = lower($2)
			`, ownerIdentity.id, r.PathValue("slug")).Scan(
				&identity.restaurantID,
				&identity.restaurantName,
				&identity.restaurantSlug,
				&identity.role,
			)
			if errors.Is(err, sql.ErrNoRows) {
				writeMappedError(w, r, controller.logger, workspaceNotFound())
				return
			}
			if err != nil {
				writeMappedError(w, r, controller.logger, fmt.Errorf("resolve owner restaurant membership: %w", err))
				return
			}
		} else {
			writeMappedError(w, r, controller.logger, workspaceNotFound())
			return
		}

		if !strings.EqualFold(identity.restaurantSlug, r.PathValue("slug")) {
			writeMappedError(w, r, controller.logger, workspaceNotFound())
			return
		}
		if !roleAllowed(identity.role, allowedRoles) {
			writeMappedError(w, r, controller.logger, workspaceForbidden())
			return
		}
		ctx := context.WithValue(r.Context(), staffIdentityContextKey{}, identity)
		ctx = context.WithValue(ctx, workspaceIdentityContextKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(staffSessionCookieName); err == nil {
			controller.staffAuth.requireStaff(resolveWorkspace).ServeHTTP(w, r)
			return
		}
		controller.ownerAuth.requireOwner(resolveWorkspace).ServeHTTP(w, r)
	})
}

type workspaceIdentityContextKey struct{}

func workspaceIdentityFromContext(ctx context.Context) (staffIdentity, bool) {
	identity, ok := ctx.Value(workspaceIdentityContextKey{}).(staffIdentity)
	return identity, ok
}

func roleAllowed(role string, allowedRoles []string) bool {
	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return true
		}
	}
	return false
}

func (controller *ownerWorkspaceController) getOverview(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}

	var workspace workspaceOverview
	workspace.ID = identity.restaurantID
	workspace.Name = identity.restaurantName
	workspace.Slug = identity.restaurantSlug
	workspace.Role = identity.role
	workspace.GuestURL = controller.guestURL(identity.restaurantSlug)
	if err := controller.db.QueryRowContext(r.Context(), `
		SELECT timezone FROM restaurants WHERE id = $1 AND is_active
	`, identity.restaurantID).Scan(&workspace.Timezone); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeMappedError(w, r, controller.logger, workspaceNotFound())
			return
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("load workspace timezone: %w", err))
		return
	}
	businessDate, err := restaurantBusinessDate(time.Now(), workspace.Timezone)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("calculate workspace business date: %w", err))
		return
	}
	err = controller.db.QueryRowContext(r.Context(), `
		SELECT r.description, r.address, r.phone,
		       r.table_queue_enabled, r.pickup_queue_enabled,
		       (SELECT count(*) FROM queue_tickets t
		        JOIN queues q ON q.id = t.queue_id AND q.restaurant_id = t.restaurant_id
		        WHERE t.restaurant_id = r.id AND q.business_date = $2
		          AND t.type = 'TABLE' AND t.status = 'WAITING' AND t.expires_at > now()),
		       (SELECT count(*) FROM queue_tickets t
		        JOIN queues q ON q.id = t.queue_id AND q.restaurant_id = t.restaurant_id
		        WHERE t.restaurant_id = r.id AND q.business_date = $2
		          AND t.type = 'TABLE' AND t.status = 'ARRIVED' AND t.expires_at > now()),
		       (SELECT count(*) FROM orders o
		        WHERE o.restaurant_id = r.id AND o.business_date = $2
		          AND o.status IN ('PENDING', 'ACCEPTED', 'PREPARING', 'READY')),
		       (SELECT count(*) FROM restaurant_tables t
		        WHERE t.restaurant_id = r.id AND t.is_active
		          AND NOT EXISTS (
		              SELECT 1 FROM table_assignments a
		              WHERE a.table_id = t.id AND a.released_at IS NULL
		          )),
		       (SELECT count(*) FROM restaurant_memberships m
		        WHERE m.restaurant_id = r.id AND m.is_active)
		FROM restaurants r
		WHERE r.id = $1 AND r.is_active
	`, identity.restaurantID, businessDate).Scan(
		&workspace.Description,
		&workspace.Address,
		&workspace.Phone,
		&workspace.TableQueueEnabled,
		&workspace.PickupQueueEnabled,
		&workspace.WaitingTables,
		&workspace.ArrivedTables,
		&workspace.OpenOrders,
		&workspace.AvailableTables,
		&workspace.ActiveStaff,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load owner workspace overview: %w", err))
		return
	}

	if err := writeSuccess(w, http.StatusOK, "Workspace loaded.", workspace); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write owner workspace response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) updateSettings(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	var body workspaceSettingsRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Description = strings.TrimSpace(body.Description)
	body.Address = strings.TrimSpace(body.Address)
	body.Phone = strings.TrimSpace(body.Phone)
	if !validTextLength(body.Name, 1, 120) ||
		len(body.Description) > 2000 ||
		len(body.Address) > 500 ||
		len(body.Phone) > 50 {
		writeMappedError(w, r, controller.logger, invalidWorkspaceSettings())
		return
	}

	_, err := controller.db.ExecContext(r.Context(), `
		UPDATE restaurants
		SET name = $1,
		    description = NULLIF($2, ''),
		    address = NULLIF($3, ''),
		    phone = NULLIF($4, ''),
		    table_queue_enabled = $5,
		    pickup_queue_enabled = $6,
		    updated_at = now()
		WHERE id = $7 AND is_active
	`, body.Name, body.Description, body.Address, body.Phone,
		body.TableQueueEnabled, body.PickupQueueEnabled, identity.restaurantID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("update restaurant workspace settings: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Restaurant settings saved.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write workspace settings response", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) getQR(w http.ResponseWriter, r *http.Request) {
	identity, ok := workspaceIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, workspaceNotFound())
		return
	}
	qr, err := qrcode.Encode(controller.guestURL(identity.restaurantSlug), qrcode.High, 1024)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate restaurant QR code: %w", err))
		return
	}
	var logoData []byte
	if err := controller.db.QueryRowContext(r.Context(), `
		SELECT logo_data FROM restaurants WHERE id = $1
	`, identity.restaurantID).Scan(&logoData); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load restaurant logo for QR code: %w", err))
		return
	}
	qr, err = addLogoToQRCode(qr, logoData)
	if err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+identity.restaurantSlug+`-qr.png"`)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(qr); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(), "write restaurant QR image", "error", err,
		)
	}
}

func (controller *ownerWorkspaceController) guestURL(slug string) string {
	base, err := url.Parse(controller.appBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "/r/" + url.PathEscape(slug)
	}
	base.Path = "/r/" + slug
	base.RawQuery = ""
	base.Fragment = ""
	return base.String()
}

func workspaceNotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    "WORKSPACE_NOT_FOUND",
		Message: "This restaurant workspace is not available to your account.",
	}
}

func workspaceForbidden() *APIError {
	return &APIError{
		Status:  http.StatusForbidden,
		Code:    "WORKSPACE_FORBIDDEN",
		Message: "Your restaurant role cannot perform this action.",
	}
}

func invalidWorkspaceSettings() *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "WORKSPACE_SETTINGS_INVALID",
		Message: "Check the restaurant name and optional details, then try again.",
	}
}
