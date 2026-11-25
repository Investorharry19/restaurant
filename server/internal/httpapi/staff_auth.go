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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"restaurant/server/internal/staffauth"
)

const staffSessionCookieName = "restaurant_staff_session"

type staffIdentity struct {
	userID         string
	userName       string
	restaurantID   string
	restaurantName string
	restaurantSlug string
	role           string
}

func (controller *staffAuthController) selectRestaurant(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	identity, ok := staffIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	var body selectRestaurantRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Slug = strings.TrimSpace(body.Slug)
	if body.Slug == "" || len(body.Slug) > 120 {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "RESTAURANT_INVALID",
			Message: "Choose a valid restaurant.",
		})
		return
	}

	tokenHash, err := staffSessionHashFromRequest(r)
	if err != nil {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	err = controller.db.QueryRowContext(r.Context(), `
		UPDATE staff_sessions s
		SET active_restaurant_id = r.id, last_seen_at = now()
		FROM restaurant_memberships m
		JOIN restaurants r ON r.id = m.restaurant_id AND r.is_active
		WHERE s.session_token_hash = $1
		  AND s.user_id = $2
		  AND s.expires_at > now()
		  AND m.user_id = s.user_id
		  AND m.restaurant_id = r.id
		  AND m.is_active
		  AND lower(r.slug) = lower($3)
		RETURNING r.id::text, r.name, r.slug, m.role
	`, tokenHash, identity.userID, body.Slug).Scan(
		&identity.restaurantID,
		&identity.restaurantName,
		&identity.restaurantSlug,
		&identity.role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, &APIError{
			Status:  http.StatusNotFound,
			Code:    "RESTAURANT_ACCESS_DENIED",
			Message: "Restaurant not found or unavailable to this account.",
		})
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("select active restaurant: %w", err))
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Active restaurant changed.", authResponse(identity)); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write selected restaurant response",
			"error", err,
		)
	}
}

type staffIdentityContextKey struct{}

type staffAuthController struct {
	db                  *sql.DB
	logger              *slog.Logger
	sessionTTL          time.Duration
	sessionCookieDomain string
	secureCookies       bool
	ipLimiter           *rateLimiter
	accountLimiter      *rateLimiter
}

func (limiter *rateLimiter) reset(key string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	delete(limiter.entries, key)
}

type staffLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type selectRestaurantRequest struct {
	Slug string `json:"slug"`
}

type staffAuthResponse struct {
	UserName       string `json:"userName"`
	RestaurantName string `json:"restaurantName"`
	RestaurantSlug string `json:"restaurantSlug"`
	Role           string `json:"role"`
}

type rateLimitEntry struct {
	count   int
	resetAt time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	limit   int
	window  time.Duration
}

func newStaffAuthController(
	db *sql.DB,
	logger *slog.Logger,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	secureCookies bool,
) *staffAuthController {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return &staffAuthController{
		db:                  db,
		logger:              logger,
		sessionTTL:          sessionTTL,
		sessionCookieDomain: sessionCookieDomain,
		secureCookies:       secureCookies,
		ipLimiter:           newRateLimiter(20, 15*time.Minute),
		accountLimiter:      newRateLimiter(8, 15*time.Minute),
	}
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
	}
}

func (limiter *rateLimiter) allow(key string) bool {
	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry, exists := limiter.entries[key]
	if !exists || now.After(entry.resetAt) {
		if len(limiter.entries) > 10000 {
			for candidate, oldEntry := range limiter.entries {
				if now.After(oldEntry.resetAt) {
					delete(limiter.entries, candidate)
				}
			}
			if len(limiter.entries) > 10000 {
				return false
			}
		}
		limiter.entries[key] = rateLimitEntry{count: 1, resetAt: now.Add(limiter.window)}
		return true
	}
	if entry.count >= limiter.limit {
		return false
	}
	entry.count++
	limiter.entries[key] = entry
	return true
}

func registerStaffAuthRoutes(mux *http.ServeMux, controller *staffAuthController) {
	mux.HandleFunc("POST /api/auth/login", controller.login)
	mux.HandleFunc("POST /api/auth/logout", controller.logout)
	mux.Handle("GET /api/auth/me", controller.requireStaff(http.HandlerFunc(controller.me)))
	mux.Handle(
		"POST /api/auth/select-restaurant",
		controller.requireStaff(http.HandlerFunc(controller.selectRestaurant)),
	)
}

func (controller *staffAuthController) login(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	var body staffLoginRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || len(email) > 320 || len(body.Password) > 1024 {
		writeMappedError(w, r, controller.logger, invalidStaffCredentials())
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

	var identity staffIdentity
	var passwordHash string
	const query = `
		SELECT u.id::text, u.name, u.password_hash,
		       r.id::text, r.name, r.slug, m.role
		FROM users u
		JOIN restaurant_memberships m
		  ON m.user_id = u.id AND m.is_active
		JOIN restaurants r
		  ON r.id = m.restaurant_id AND r.is_active
		WHERE lower(u.email) = $1 AND u.is_active
		ORDER BY m.created_at, m.id
		LIMIT 1
	`
	err := controller.db.QueryRowContext(r.Context(), query, email).Scan(
		&identity.userID,
		&identity.userName,
		&passwordHash,
		&identity.restaurantID,
		&identity.restaurantName,
		&identity.restaurantSlug,
		&identity.role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		verifyDummyPassword(body.Password)
		writeMappedError(w, r, controller.logger, invalidStaffCredentials())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load staff account: %w", err))
		return
	}
	if !staffauth.VerifyPassword(passwordHash, []byte(body.Password)) {
		writeMappedError(w, r, controller.logger, invalidStaffCredentials())
		return
	}
	controller.accountLimiter.reset(ip + "\x00" + email)

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("generate staff session token: %w", err))
		return
	}
	tokenString := base64.RawURLEncoding.EncodeToString(token)
	tokenHash := sha256.Sum256(token)
	expiresAt := time.Now().Add(controller.sessionTTL)
	if _, err := controller.db.ExecContext(
		r.Context(),
		`INSERT INTO staff_sessions (user_id, session_token_hash, active_restaurant_id, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		identity.userID,
		tokenHash[:],
		identity.restaurantID,
		expiresAt,
	); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create staff session: %w", err))
		return
	}

	controller.setSessionCookie(w, tokenString, expiresAt)
	if err := writeSuccess(w, http.StatusOK, "Signed in.", authResponse(identity)); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write staff login response",
			"error", err,
		)
	}
}

func (controller *staffAuthController) logout(w http.ResponseWriter, r *http.Request) {
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	if cookie, err := r.Cookie(staffSessionCookieName); err == nil {
		if token, decodeErr := base64.RawURLEncoding.DecodeString(cookie.Value); decodeErr == nil && len(token) == 32 {
			tokenHash := sha256.Sum256(token)
			if _, err := controller.db.ExecContext(
				r.Context(),
				`DELETE FROM staff_sessions WHERE session_token_hash = $1`,
				tokenHash[:],
			); err != nil {
				writeMappedError(w, r, controller.logger, fmt.Errorf("delete staff session: %w", err))
				return
			}
		}
	}
	controller.clearSessionCookie(w)
	if err := writeSuccess(w, http.StatusOK, "Signed out.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write staff logout response",
			"error", err,
		)
	}
}

func (controller *staffAuthController) me(w http.ResponseWriter, r *http.Request) {
	identity, ok := r.Context().Value(staffIdentityContextKey{}).(staffIdentity)
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedStaff())
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Staff session loaded.", authResponse(identity)); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write staff session response",
			"error", err,
		)
	}
}

func (controller *staffAuthController) requireStaff(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenHash, err := staffSessionHashFromRequest(r)
		if err != nil {
			writeMappedError(w, r, controller.logger, unauthorizedStaff())
			return
		}
		var identity staffIdentity
		err = controller.db.QueryRowContext(r.Context(), `
			SELECT u.id::text, u.name, r.id::text, r.name, r.slug, m.role
			FROM staff_sessions s
			JOIN users u ON u.id = s.user_id AND u.is_active
			JOIN restaurant_memberships m
			  ON m.user_id = u.id
			 AND m.restaurant_id = s.active_restaurant_id
			 AND m.is_active
			JOIN restaurants r ON r.id = m.restaurant_id AND r.is_active
			WHERE s.session_token_hash = $1 AND s.expires_at > now()
		`, tokenHash).Scan(
			&identity.userID,
			&identity.userName,
			&identity.restaurantID,
			&identity.restaurantName,
			&identity.restaurantSlug,
			&identity.role,
		)
		if errors.Is(err, sql.ErrNoRows) {
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedStaff())
			return
		}
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("resolve staff session: %w", err))
			return
		}
		ctx := context.WithValue(r.Context(), staffIdentityContextKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func staffSessionHashFromRequest(r *http.Request) ([]byte, error) {
	cookie, err := r.Cookie(staffSessionCookieName)
	if err != nil {
		return nil, err
	}
	token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(token) != 32 {
		return nil, fmt.Errorf("invalid staff session token")
	}
	tokenHash := sha256.Sum256(token)
	return tokenHash[:], nil
}

func (controller *staffAuthController) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     staffSessionCookieName,
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

func (controller *staffAuthController) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     staffSessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   controller.sessionCookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   controller.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func validateBrowserMutation(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		return true
	}
	writeMappedError(w, r, logger, &APIError{
		Status:  http.StatusForbidden,
		Code:    "CSRF_REJECTED",
		Message: "The request could not be verified.",
	})
	return false
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func invalidStaffCredentials() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "AUTH_INVALID_CREDENTIALS",
		Message: "Email or password is incorrect.",
	}
}

func unauthorizedStaff() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "AUTH_REQUIRED",
		Message: "Sign in to continue.",
	}
}

func authResponse(identity staffIdentity) staffAuthResponse {
	return staffAuthResponse{
		UserName:       identity.userName,
		RestaurantName: identity.restaurantName,
		RestaurantSlug: identity.restaurantSlug,
		Role:           identity.role,
	}
}

var dummyPasswordHashOnce sync.Once
var dummyPasswordHash string

func verifyDummyPassword(password string) {
	dummyPasswordHashOnce.Do(func() {
		dummyPasswordHash = staffauth.DummyPasswordHash()
	})
	staffauth.VerifyPassword(dummyPasswordHash, []byte(password))
}
