package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"restaurant/server/internal/config"
	"restaurant/server/internal/database"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close database", "error", err)
		}
	}()

	foundMigrations, err := database.Migrate(ctx, db, cfg.MigrationsDir)
	if err != nil {
		return err
	}
	if foundMigrations {
		slog.Info("database migration step completed")
	} else {
		slog.Info("no schema migrations are defined yet; the migration runner is ready")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
