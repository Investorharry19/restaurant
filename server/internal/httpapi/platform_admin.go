package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"restaurant/server/internal/staffauth"
)

const platformAdminSessionCookieName = "restaurant_platform_admin_session"

type platformAdmin struct {
	id   string
	name string
}

type platformAdminContextKey struct{}

type platformAdminAuthController struct {
	db                  *sql.DB
	logger              *slog.Logger
	sessionTTL          time.Duration
	sessionCookieDomain string
	secureCookies       bool
	ipLimiter           *rateLimiter
	accountLimiter      *rateLimiter
}

type platformAdminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type platformAdminSessionResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newPlatformAdminAuthController(
	db *sql.DB,
	logger *slog.Logger,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	secureCookies bool,
) *platformAdminAuthController {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return &platformAdminAuthController{
		db:                  db,
		logger:              logger,
		sessionTTL:          sessionTTL,
		sessionCookieDomain: sessionCookieDomain,
		secureCookies:       secureCookies,
		ipLimiter:           newRateLimiter(20, 15*time.Minute),
		accountLimiter:      newRateLimiter(8, 15*time.Minute),
	}
}

func registerPlatformAdminAuthRoutes(
	mux *http.ServeMux,
	controller *platformAdminAuthController,
) {
	mux.HandleFunc("POST /api/platform/auth/login", controller.login)
	mux.HandleFunc("POST /api/platform/auth/logout", controller.logout)
	mux.Handle(
		"GET /api/platform/auth/me",
		controller.requireAdmin(http.HandlerFunc(controller.me)),
	)
}

func (controller *platformAdminAuthController) login(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}

	var body platformAdminLoginRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || len(email) > 320 || len(body.Password) > 1024 {
		writeMappedError(w, r, controller.logger, invalidPlatformAdminCredentials())
		return
	}
	ip := requestIP(r)
	if !controller.ipLimiter.allow(ip) ||
		!controller.accountLimiter.allow(ip+"\x00"+email) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusTooManyRequests,
			Code:    "AUTH_RATE_LIMITED",
			Message: "Too many sign-in attempts. Try again later.",
		})
		return
	}

	var admin platformAdmin
	var response platformAdminSessionResponse
	var passwordHash string
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT id::text, name, email, password_hash
		FROM platform_admins
		WHERE lower(email) = $1 AND is_active
	`, email).Scan(&admin.id, &admin.name, &response.Email, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		verifyDummyPassword(body.Password)
		writeMappedError(w, r, controller.logger, invalidPlatformAdminCredentials())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load platform administrator: %w", err))
		return
	}
	if !staffauth.VerifyPassword(passwordHash, []byte(body.Password)) {
		writeMappedError(w, r, controller.logger, invalidPlatformAdminCredentials())
		return
	}
	controller.accountLimiter.reset(ip + "\x00" + email)

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate platform admin session token: %w", err))
		return
	}
	tokenString := base64.RawURLEncoding.EncodeToString(token)
	tokenHash := sha256.Sum256(token)
	clear(token)
	expiresAt := time.Now().Add(controller.sessionTTL)
	if _, err := controller.db.ExecContext(r.Context(), `
		INSERT INTO platform_admin_sessions (admin_id, session_token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, admin.id, tokenHash[:], expiresAt); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create platform admin session: %w", err))
		return
	}
	if err := controller.recordAccess(r.Context(), admin.id, "PLATFORM_ADMIN_SIGNED_IN"); err != nil {
		if _, deleteErr := controller.db.ExecContext(
			r.Context(),
			`DELETE FROM platform_admin_sessions WHERE session_token_hash = $1`,
			tokenHash[:],
		); deleteErr != nil {
			controller.logger.ErrorContext(
				r.Context(),
				"delete unaudited platform admin session",
				"error", deleteErr,
			)
		}
		writeMappedError(w, r, controller.logger, fmt.Errorf("audit platform admin sign-in: %w", err))
		return
	}

	response.Name = admin.name
	controller.setSessionCookie(w, tokenString, expiresAt)
	if err := writeSuccess(w, http.StatusOK, "Signed in.", response); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write platform admin login response",
			"error", err,
		)
	}
}

func (controller *platformAdminAuthController) logout(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	if cookie, err := r.Cookie(platformAdminSessionCookieName); err == nil {
		if token, decodeErr := base64.RawURLEncoding.DecodeString(cookie.Value); decodeErr == nil &&
			len(token) == 32 {
			tokenHash := sha256.Sum256(token)
			clear(token)
			if _, err := controller.db.ExecContext(
				r.Context(),
				`DELETE FROM platform_admin_sessions WHERE session_token_hash = $1`,
				tokenHash[:],
			); err != nil {
				writeMappedError(w, r, controller.logger, fmt.Errorf("delete platform admin session: %w", err))
				return
			}
		}
	}
	controller.clearSessionCookie(w)
	if err := writeSuccess(w, http.StatusOK, "Signed out.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write platform admin logout response",
			"error", err,
		)
	}
}

func (controller *platformAdminAuthController) me(w http.ResponseWriter, r *http.Request) {
	admin, ok := platformAdminFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
		return
	}

	var email string
	if err := controller.db.QueryRowContext(
		r.Context(),
		`SELECT email FROM platform_admins WHERE id = $1`,
		admin.id,
	).Scan(&email); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load platform admin session: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Platform admin session loaded.", platformAdminSessionResponse{
		Name:  admin.name,
		Email: email,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write platform admin session response",
			"error", err,
		)
	}
}

func (controller *platformAdminAuthController) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(platformAdminSessionCookieName)
		if err != nil {
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
			return
		}
		token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		if err != nil || len(token) != 32 {
			clear(token)
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
			return
		}
		tokenHash := sha256.Sum256(token)
		clear(token)

		var admin platformAdmin
		err = controller.db.QueryRowContext(r.Context(), `
			UPDATE platform_admin_sessions s
			SET last_seen_at = now()
			FROM platform_admins a
			WHERE s.session_token_hash = $1
			  AND s.admin_id = a.id
			  AND s.expires_at > now()
			  AND a.is_active
			RETURNING a.id::text, a.name
		`, tokenHash[:]).Scan(&admin.id, &admin.name)
		if errors.Is(err, sql.ErrNoRows) {
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedPlatformAdmin())
			return
		}
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("resolve platform admin session: %w", err))
			return
		}

		ctx := context.WithValue(r.Context(), platformAdminContextKey{}, admin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (controller *platformAdminAuthController) recordAccess(
	ctx context.Context,
	adminID string,
	action string,
) error {
	_, err := controller.db.ExecContext(ctx, `
		INSERT INTO platform_audit_logs (admin_id, action)
		VALUES ($1, $2)
	`, adminID, action)
	return err
}

func (controller *platformAdminAuthController) setSessionCookie(
	w http.ResponseWriter,
	token string,
	expiresAt time.Time,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     platformAdminSessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   controller.sessionCookieDomain,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   controller.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (controller *platformAdminAuthController) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     platformAdminSessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   controller.sessionCookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   controller.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func platformAdminFromContext(ctx context.Context) (platformAdmin, bool) {
	admin, ok := ctx.Value(platformAdminContextKey{}).(platformAdmin)
	return admin, ok
}

func invalidPlatformAdminCredentials() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "PLATFORM_AUTH_INVALID_CREDENTIALS",
		Message: "Email or password is incorrect.",
	}
}

func unauthorizedPlatformAdmin() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "PLATFORM_AUTH_REQUIRED",
		Message: "Sign in as a platform administrator to continue.",
	}
}
