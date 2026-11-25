package httpapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"restaurant/server/internal/realtime"
)

type PushSettings struct {
	VAPIDPublicKey     string
	TokenEncryptionKey []byte
}

func NewHandler(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
) http.Handler {
	return NewHandlerWithSessionSettings(
		db,
		logger,
		allowedOrigins,
		production,
		24*time.Hour,
		"",
	)
}

func NewHandlerWithSessionSettings(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
	sessionTTL time.Duration,
	sessionCookieDomain string,
) http.Handler {
	return NewHandlerWithEmailSender(
		db,
		logger,
		allowedOrigins,
		production,
		sessionTTL,
		sessionCookieDomain,
		"",
		nil,
	)
}

func NewHandlerWithEmailSender(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	appBaseURL string,
	emailSender VerificationEmailSender,
) http.Handler {
	return NewHandlerWithRealtime(
		db,
		logger,
		allowedOrigins,
		production,
		sessionTTL,
		sessionCookieDomain,
		appBaseURL,
		emailSender,
		realtime.NewHub(),
	)
}

func NewHandlerWithRealtime(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	appBaseURL string,
	emailSender VerificationEmailSender,
	eventHub *realtime.Hub,
) http.Handler {
	return NewHandlerWithPush(
		db,
		logger,
		allowedOrigins,
		production,
		sessionTTL,
		sessionCookieDomain,
		appBaseURL,
		emailSender,
		eventHub,
		PushSettings{},
	)
}

func NewHandlerWithPush(
	db *sql.DB,
	logger *slog.Logger,
	allowedOrigins []string,
	production bool,
	sessionTTL time.Duration,
	sessionCookieDomain string,
	appBaseURL string,
	emailSender VerificationEmailSender,
	eventHub *realtime.Hub,
	pushSettings PushSettings,
) http.Handler {
	if eventHub == nil {
		eventHub = realtime.NewHub()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(logger))
	mux.HandleFunc("GET /readyz", readinessHandler(logger, db))
	authController := newStaffAuthController(
		db,
		logger,
		sessionTTL,
		sessionCookieDomain,
		production,
	)
	platformAdminAuthController := newPlatformAdminAuthController(
		db,
		logger,
		sessionTTL,
		sessionCookieDomain,
		production,
	)
	ownerAuthController := newOwnerAuthController(
		db,
		logger,
		emailSender,
		appBaseURL,
		sessionTTL,
		sessionCookieDomain,
		production,
	)
	registerStaffAuthRoutes(mux, authController)
	registerStaffOrderRoutes(mux, authController)
	registerPlatformAdminAuthRoutes(mux, platformAdminAuthController)
	registerOwnerAuthRoutes(mux, ownerAuthController)
	registerOwnerWorkspaceRoutes(
		mux,
		newOwnerWorkspaceController(
			db,
			logger,
			ownerAuthController,
			authController,
			appBaseURL,
			eventHub,
		),
	)
	registerPlatformApplicationRoutes(
		mux,
		newPublicApplicationController(db, logger),
		newPlatformApplicationController(db, logger),
		platformAdminAuthController,
		ownerAuthController,
	)
	registerPublicRestaurantRoutes(
		mux,
		postgresPublicRestaurantReader{db: db},
		logger,
	)
	registerCustomerTicketRoutes(
		mux,
		newPublicTicketController(db, logger, eventHub, pushSettings),
	)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeMappedError(
			w,
			r,
			logger,
			&APIError{
				Status:  http.StatusNotFound,
				Code:    "NOT_FOUND",
				Message: "API route not found.",
			},
		)
	})

	return middleware(mux, logger, allowedOrigins, production)
}
