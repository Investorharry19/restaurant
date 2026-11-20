package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"restaurant/server/internal/config"
	"restaurant/server/internal/database"
	"restaurant/server/internal/httpapi"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := database.Open(startupCtx, cfg.DatabaseURL)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(db, logger, cfg.CORSOrigins, cfg.AppEnv == "production"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if cfg.TLSCertFile != "" {
		server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTPAddr, err)
	}

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErrors := make(chan error, 1)
	go func() {
		if cfg.TLSCertFile != "" {
			serveErrors <- server.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		serveErrors <- server.Serve(listener)
	}()

	logger.Info("HTTP server started", "address", cfg.HTTPAddr, "environment", cfg.AppEnv)

	select {
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			if closeErr := server.Close(); closeErr != nil {
				return fmt.Errorf("shut down HTTP server: %w (force close failed: %v)", err, closeErr)
			}
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		if err := <-serveErrors; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP after shutdown: %w", err)
		}
		logger.Info("HTTP server stopped")
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("API server failed", "error", err)
		os.Exit(1)
	}
}
