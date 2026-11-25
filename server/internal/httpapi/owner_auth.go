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
	"net/mail"
	"net/url"
	"strings"
	"time"

	"restaurant/server/internal/staffauth"
)

const (
	ownerSessionCookieName = "restaurant_owner_session"
	ownerVerificationTTL   = 24 * time.Hour
)

type ownerIdentity struct {
	id    string
	name  string
	email string
}

type ownerIdentityContextKey struct{}

type ownerAuthController struct {
	db                  *sql.DB
	logger              *slog.Logger
	emailSender         VerificationEmailSender
	appBaseURL          string
	sessionTTL          time.Duration
	sessionCookieDomain string
	secureCookies       bool
	ipLimiter           *rateLimiter
	accountLimiter      *rateLimiter
}

type ownerRegistrationRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ownerVerificationRequest struct {
	Token string `json:"token"`
}

type ownerEmailRequest struct {
	Email string `json:"email"`
}

type ownerLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ownerSessionResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newOwnerAuthController(
	db *sql.DB,
	logger *slog.Logger,
	emailSender VerificationEmailSender,
	appBaseURL string,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	secureCookies bool,
) *ownerAuthController {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return &ownerAuthController{
		db:                  db,
		logger:              logger,
		emailSender:         emailSender,
		appBaseURL:          appBaseURL,
		sessionTTL:          sessionTTL,
		sessionCookieDomain: sessionCookieDomain,
		secureCookies:       secureCookies,
		ipLimiter:           newRateLimiter(20, 15*time.Minute),
		accountLimiter:      newRateLimiter(8, 15*time.Minute),
	}
}

func registerOwnerAuthRoutes(mux *http.ServeMux, controller *ownerAuthController) {
	mux.HandleFunc("POST /api/public/accounts", controller.register)
	mux.HandleFunc("POST /api/public/accounts/verify", controller.verifyEmail)
	mux.HandleFunc("POST /api/public/accounts/verification/resend", controller.resendVerification)
	mux.HandleFunc("POST /api/owner/auth/login", controller.login)
	mux.HandleFunc("POST /api/owner/auth/logout", controller.logout)
	mux.Handle(
		"GET /api/owner/auth/me",
		controller.requireOwner(http.HandlerFunc(controller.me)),
	)
}

func (controller *ownerAuthController) register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	var body ownerRegistrationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if err := validateOwnerRegistration(body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	ip := requestIP(r)
	if !controller.ipLimiter.allow(ip) ||
		!controller.accountLimiter.allow(ip+"\x00"+body.Email) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}

	password := []byte(body.Password)
	body.Password = ""
	passwordHash, err := staffauth.HashPassword(password)
	clear(password)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("hash owner account password: %w", err))
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin owner registration: %w", err))
		return
	}
	defer tx.Rollback()

	var userID string
	var verifiedAt sql.NullTime
	var isActive bool
	err = tx.QueryRowContext(r.Context(), `
		SELECT id::text, email_verified_at, is_active
		FROM users
		WHERE lower(email) = $1
		FOR UPDATE
	`, body.Email).Scan(&userID, &verifiedAt, &isActive)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = tx.QueryRowContext(r.Context(), `
			INSERT INTO users (name, email, password_hash, email_verified_at)
			VALUES ($1, $2, $3, NULL)
			RETURNING id::text
		`, body.Name, body.Email, passwordHash).Scan(&userID)
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("create owner account: %w", err))
			return
		}
		isActive = true
	case err != nil:
		writeMappedError(w, r, controller.logger, fmt.Errorf("load owner account: %w", err))
		return
	}

	shouldSend := isActive && !verifiedAt.Valid
	var token string
	if shouldSend {
		token, err = createOwnerVerificationToken(r.Context(), tx, userID)
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("create owner verification token: %w", err))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit owner registration: %w", err))
		return
	}

	if shouldSend {
		if err := controller.sendVerificationEmail(r.Context(), body.Email, token); err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("send owner verification email: %w", err))
			return
		}
	}
	if err := writeSuccess(w, http.StatusAccepted, "If this email can be registered, a verification link is on its way.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write owner registration response",
			"error", err,
		)
	}
}

func (controller *ownerAuthController) resendVerification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	var body ownerEmailRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if !validEmailAddress(body.Email) {
		writeMappedError(w, r, controller.logger, invalidOwnerRegistration("Enter a valid email address."))
		return
	}
	ip := requestIP(r)
	if !controller.ipLimiter.allow(ip) ||
		!controller.accountLimiter.allow(ip+"\x00"+body.Email) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}

	var userID string
	var verifiedAt sql.NullTime
	var isActive bool
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT id::text, email_verified_at, is_active
		FROM users
		WHERE lower(email) = $1
	`, body.Email).Scan(&userID, &verifiedAt, &isActive)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (verifiedAt.Valid || !isActive)) {
		writeOwnerVerificationAcknowledgement(w, r, controller.logger)
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load account for verification resend: %w", err))
		return
	}

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin verification resend: %w", err))
		return
	}
	defer tx.Rollback()
	token, err := createOwnerVerificationToken(r.Context(), tx, userID)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create resent owner verification token: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit verification resend: %w", err))
		return
	}
	if err := controller.sendVerificationEmail(r.Context(), body.Email, token); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("resend owner verification email: %w", err))
		return
	}
	writeOwnerVerificationAcknowledgement(w, r, controller.logger)
}

func (controller *ownerAuthController) verifyEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	var body ownerVerificationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	if !applicationStatusPattern.MatchString(body.Token) {
		writeMappedError(w, r, controller.logger, invalidOwnerVerificationToken())
		return
	}
	token, err := base64.RawURLEncoding.DecodeString(body.Token)
	if err != nil || len(token) != 32 {
		clear(token)
		writeMappedError(w, r, controller.logger, invalidOwnerVerificationToken())
		return
	}
	tokenHash := sha256.Sum256(token)
	clear(token)

	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin owner email verification: %w", err))
		return
	}
	defer tx.Rollback()

	var identity ownerIdentity
	err = tx.QueryRowContext(r.Context(), `
		SELECT u.id::text, u.name, u.email
		FROM owner_email_verification_tokens t
		JOIN users u ON u.id = t.user_id AND u.is_active
		WHERE t.token_hash = $1 AND t.expires_at > now()
		FOR UPDATE OF t, u
	`, tokenHash[:]).Scan(&identity.id, &identity.name, &identity.email)
	if errors.Is(err, sql.ErrNoRows) {
		writeMappedError(w, r, controller.logger, invalidOwnerVerificationToken())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load owner verification token: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE users SET email_verified_at = COALESCE(email_verified_at, now())
		WHERE id = $1
	`, identity.id); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("verify owner email address: %w", err))
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		DELETE FROM owner_email_verification_tokens WHERE user_id = $1
	`, identity.id); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("consume owner verification token: %w", err))
		return
	}
	sessionToken, expiresAt, err := createOwnerSession(r.Context(), tx, identity.id, controller.sessionTTL)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create verified owner session: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit owner email verification: %w", err))
		return
	}

	controller.setSessionCookie(w, sessionToken, expiresAt)
	if err := writeSuccess(w, http.StatusOK, "Email verified.", ownerSessionResponse{
		Name:  identity.name,
		Email: identity.email,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write owner verification response",
			"error", err,
		)
	}
}

func (controller *ownerAuthController) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	var body ownerLoginRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeMappedError(w, r, controller.logger, err)
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if body.Email == "" || len(body.Email) > 320 || len(body.Password) > 1024 {
		writeMappedError(w, r, controller.logger, invalidOwnerCredentials())
		return
	}
	ip := requestIP(r)
	if !controller.ipLimiter.allow(ip) ||
		!controller.accountLimiter.allow(ip+"\x00"+body.Email) {
		writeMappedError(w, r, controller.logger, ownerRateLimited())
		return
	}

	var identity ownerIdentity
	var passwordHash string
	var verifiedAt sql.NullTime
	err := controller.db.QueryRowContext(r.Context(), `
		SELECT id::text, name, email, password_hash, email_verified_at
		FROM users
		WHERE lower(email) = $1 AND is_active
	`, body.Email).Scan(
		&identity.id,
		&identity.name,
		&identity.email,
		&passwordHash,
		&verifiedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		verifyDummyPassword(body.Password)
		writeMappedError(w, r, controller.logger, invalidOwnerCredentials())
		return
	}
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("load owner account for sign-in: %w", err))
		return
	}
	if !staffauth.VerifyPassword(passwordHash, []byte(body.Password)) {
		writeMappedError(w, r, controller.logger, invalidOwnerCredentials())
		return
	}
	if !verifiedAt.Valid {
		writeMappedError(w, r, controller.logger, ownerEmailNotVerified())
		return
	}

	controller.accountLimiter.reset(ip + "\x00" + body.Email)
	tx, err := controller.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("begin owner sign-in: %w", err))
		return
	}
	defer tx.Rollback()
	sessionToken, expiresAt, err := createOwnerSession(r.Context(), tx, identity.id, controller.sessionTTL)
	if err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("create owner session: %w", err))
		return
	}
	if err := tx.Commit(); err != nil {
		writeMappedError(w, r, controller.logger, fmt.Errorf("commit owner sign-in: %w", err))
		return
	}

	controller.setSessionCookie(w, sessionToken, expiresAt)
	if err := writeSuccess(w, http.StatusOK, "Signed in.", ownerSessionResponse{
		Name:  identity.name,
		Email: identity.email,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write owner sign-in response",
			"error", err,
		)
	}
}

func (controller *ownerAuthController) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !validateBrowserMutation(w, r, controller.logger) {
		return
	}
	if cookie, err := r.Cookie(ownerSessionCookieName); err == nil {
		token, decodeErr := base64.RawURLEncoding.DecodeString(cookie.Value)
		if decodeErr == nil && len(token) == 32 {
			tokenHash := sha256.Sum256(token)
			clear(token)
			if _, err := controller.db.ExecContext(
				r.Context(),
				`DELETE FROM owner_sessions WHERE session_token_hash = $1`,
				tokenHash[:],
			); err != nil {
				writeMappedError(w, r, controller.logger, fmt.Errorf("delete owner session: %w", err))
				return
			}
		} else {
			clear(token)
		}
	}
	controller.clearSessionCookie(w)
	if err := writeSuccess(w, http.StatusOK, "Signed out.", struct{}{}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write owner sign-out response",
			"error", err,
		)
	}
}

func (controller *ownerAuthController) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, ok := ownerIdentityFromContext(r.Context())
	if !ok {
		writeMappedError(w, r, controller.logger, unauthorizedOwner())
		return
	}
	if err := writeSuccess(w, http.StatusOK, "Owner account loaded.", ownerSessionResponse{
		Name:  identity.name,
		Email: identity.email,
	}); err != nil {
		loggerFromContext(r.Context(), controller.logger).ErrorContext(
			r.Context(),
			"write owner account response",
			"error", err,
		)
	}
}

func (controller *ownerAuthController) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		cookie, err := r.Cookie(ownerSessionCookieName)
		if err != nil {
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedOwner())
			return
		}
		token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		if err != nil || len(token) != 32 {
			clear(token)
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedOwner())
			return
		}
		tokenHash := sha256.Sum256(token)
		clear(token)

		var identity ownerIdentity
		err = controller.db.QueryRowContext(r.Context(), `
			UPDATE owner_sessions s
			SET last_seen_at = now()
			FROM users u
			WHERE s.session_token_hash = $1
			  AND s.user_id = u.id
			  AND s.expires_at > now()
			  AND u.is_active
			  AND u.email_verified_at IS NOT NULL
			RETURNING u.id::text, u.name, u.email
		`, tokenHash[:]).Scan(&identity.id, &identity.name, &identity.email)
		if errors.Is(err, sql.ErrNoRows) {
			controller.clearSessionCookie(w)
			writeMappedError(w, r, controller.logger, unauthorizedOwner())
			return
		}
		if err != nil {
			writeMappedError(w, r, controller.logger, fmt.Errorf("resolve owner session: %w", err))
			return
		}

		ctx := context.WithValue(r.Context(), ownerIdentityContextKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (controller *ownerAuthController) sendVerificationEmail(
	ctx context.Context,
	email string,
	token string,
) error {
	if controller.emailSender == nil {
		return &APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "EMAIL_DELIVERY_UNAVAILABLE",
			Message: "Email verification is not configured. Try again later.",
		}
	}
	verifyURL, err := url.Parse(controller.appBaseURL)
	if err != nil {
		return fmt.Errorf("parse application base URL: %w", err)
	}
	verifyURL.Path = "/verify-email"
	query := verifyURL.Query()
	query.Set("token", token)
	verifyURL.RawQuery = query.Encode()
	if err := controller.emailSender.SendVerificationEmail(ctx, email, verifyURL.String()); err != nil {
		return &APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "EMAIL_DELIVERY_UNAVAILABLE",
			Message: "We couldn't send the verification email. Try again shortly.",
			Cause:   err,
		}
	}
	return nil
}

func (controller *ownerAuthController) setSessionCookie(
	w http.ResponseWriter,
	token string,
	expiresAt time.Time,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     ownerSessionCookieName,
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

func (controller *ownerAuthController) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     ownerSessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   controller.sessionCookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   controller.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func ownerIdentityFromContext(ctx context.Context) (ownerIdentity, bool) {
	identity, ok := ctx.Value(ownerIdentityContextKey{}).(ownerIdentity)
	return identity, ok
}

func validateOwnerRegistration(body ownerRegistrationRequest) error {
	if !validTextLength(body.Name, 1, 120) {
		return invalidOwnerRegistration("Enter a name of up to 120 characters.")
	}
	if !validEmailAddress(body.Email) {
		return invalidOwnerRegistration("Enter a valid email address.")
	}
	if len(body.Password) < 12 || len(body.Password) > 1024 ||
		strings.TrimSpace(body.Password) == "" {
		return invalidOwnerRegistration("Choose a password between 12 and 1024 characters.")
	}
	return nil
}

func validEmailAddress(value string) bool {
	if len(value) < 3 || len(value) > 320 {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

func createOwnerVerificationToken(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
) (string, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", fmt.Errorf("generate owner verification token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256(rawToken)
	clear(rawToken)

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM owner_email_verification_tokens WHERE user_id = $1
	`, userID); err != nil {
		return "", fmt.Errorf("replace owner verification token: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO owner_email_verification_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash[:], time.Now().Add(ownerVerificationTTL)); err != nil {
		return "", fmt.Errorf("save owner verification token: %w", err)
	}
	return token, nil
}

func createOwnerSession(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	sessionTTL time.Duration,
) (string, time.Time, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", time.Time{}, fmt.Errorf("generate owner session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256(rawToken)
	clear(rawToken)
	expiresAt := time.Now().Add(sessionTTL)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO owner_sessions (user_id, session_token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash[:], expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("save owner session: %w", err)
	}
	return token, expiresAt, nil
}

func invalidOwnerRegistration(message string) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "OWNER_ACCOUNT_INVALID",
		Message: message,
	}
}

func invalidOwnerCredentials() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "OWNER_AUTH_INVALID_CREDENTIALS",
		Message: "Email or password is incorrect.",
	}
}

func ownerEmailNotVerified() *APIError {
	return &APIError{
		Status:  http.StatusForbidden,
		Code:    "OWNER_EMAIL_NOT_VERIFIED",
		Message: "Verify your email address before signing in.",
	}
}

func unauthorizedOwner() *APIError {
	return &APIError{
		Status:  http.StatusUnauthorized,
		Code:    "OWNER_AUTH_REQUIRED",
		Message: "Sign in with a verified owner account to continue.",
	}
}

func invalidOwnerVerificationToken() *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "OWNER_VERIFICATION_INVALID",
		Message: "This verification link is invalid or has expired. Request a new one and try again.",
	}
}

func ownerRateLimited() *APIError {
	return &APIError{
		Status:  http.StatusTooManyRequests,
		Code:    "OWNER_AUTH_RATE_LIMITED",
		Message: "Too many account requests. Try again later.",
	}
}

func writeOwnerVerificationAcknowledgement(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
) {
	if err := writeSuccess(
		w,
		http.StatusAccepted,
		"If this account needs verification, a link is on its way.",
		struct{}{},
	); err != nil {
		loggerFromContext(r.Context(), logger).ErrorContext(
			r.Context(),
			"write owner verification acknowledgement",
			"error", err,
		)
	}
}
