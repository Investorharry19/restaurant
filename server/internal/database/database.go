package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("ping PostgreSQL: %w (close failed: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB, migrationsDir string) (bool, error) {
	if err := goose.SetDialect("postgres"); err != nil {
		return false, fmt.Errorf("configure PostgreSQL migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		if errors.Is(err, goose.ErrNoMigrationFiles) {
			return false, nil
		}
		return false, fmt.Errorf("apply database migrations: %w", err)
	}
	return true, nil
}
