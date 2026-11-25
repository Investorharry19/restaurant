package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/term"

	"restaurant/server/internal/config"
	"restaurant/server/internal/database"
	"restaurant/server/internal/staffauth"
)

var restaurantSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	password, err := loadBootstrapPassword(func() ([]byte, error) {
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			return nil, fmt.Errorf("BOOTSTRAP_PASSWORD must be set when stdin is not a terminal")
		}
		fmt.Print("Owner password (12-1024 bytes): ")
		password, err := term.ReadPassword(fd)
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("read owner password: %w", err)
		}
		return password, nil
	})
	if err != nil {
		return err
	}
	defer clear(password)

	reader := bufio.NewReader(os.Stdin)
	restaurantName, err := prompt(reader, "Restaurant name")
	if err != nil {
		return err
	}
	slug, err := prompt(reader, "Restaurant slug")
	if err != nil {
		return err
	}
	ownerName, err := prompt(reader, "Owner name")
	if err != nil {
		return err
	}
	email, err := prompt(reader, "Owner email")
	if err != nil {
		return err
	}
	tableQueueEnabled, err := promptYesNo(reader, "Enable the table waitlist? (y/n)")
	if err != nil {
		return err
	}
	pickupQueueEnabled, err := promptYesNo(reader, "Enable pickup tickets? (y/n)")
	if err != nil {
		return err
	}
	restaurantName = strings.TrimSpace(restaurantName)
	slug = strings.TrimSpace(slug)
	ownerName = strings.TrimSpace(ownerName)
	email = strings.ToLower(strings.TrimSpace(email))
	if restaurantName == "" || len(restaurantName) > 120 {
		return fmt.Errorf("restaurant name must be between 1 and 120 characters")
	}
	if !restaurantSlugPattern.MatchString(slug) {
		return fmt.Errorf("restaurant slug must contain lowercase letters, digits, and single hyphens")
	}
	if ownerName == "" || len(ownerName) > 120 {
		return fmt.Errorf("owner name must be between 1 and 120 characters")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return fmt.Errorf("owner email is invalid")
	}
	passwordHash, err := staffauth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash owner password: %w", err)
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

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin owner bootstrap: %w", err)
	}
	defer tx.Rollback()

	var restaurantID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO restaurants (name, slug, table_queue_enabled, pickup_queue_enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, restaurantName, slug, tableQueueEnabled, pickupQueueEnabled).Scan(&restaurantID); err != nil {
		return fmt.Errorf("create restaurant: %w", err)
	}
	var userID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id::text
	`, ownerName, email, passwordHash).Scan(&userID); err != nil {
		return fmt.Errorf("create owner account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO restaurant_memberships (restaurant_id, user_id, role)
		VALUES ($1, $2, 'OWNER')
	`, restaurantID, userID); err != nil {
		return fmt.Errorf("create owner membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit owner bootstrap: %w", err)
	}

	fmt.Printf("Created owner account for restaurant %q (%s).\n", restaurantName, slug)
	return nil
}

func loadBootstrapPassword(readFromTerminal func() ([]byte, error)) ([]byte, error) {
	passwordValue, ok := os.LookupEnv("BOOTSTRAP_PASSWORD")
	if ok {
		if err := os.Unsetenv("BOOTSTRAP_PASSWORD"); err != nil {
			return nil, fmt.Errorf("clear bootstrap password environment variable: %w", err)
		}
	}

	var password []byte
	if ok {
		password = []byte(passwordValue)
	} else {
		var err error
		password, err = readFromTerminal()
		if err != nil {
			return nil, err
		}
	}

	if len(bytes.TrimSpace(password)) == 0 {
		clear(password)
		return nil, fmt.Errorf("owner password must not be blank")
	}
	if len(password) < 12 || len(password) > 1024 {
		clear(password)
		return nil, fmt.Errorf("owner password must be between 12 and 1024 bytes")
	}

	return password, nil
}

func prompt(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %s: %w", label, err)
	}
	return strings.TrimSpace(value), nil
}

func promptYesNo(reader *bufio.Reader, label string) (bool, error) {
	value, err := prompt(reader, label)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(value) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("answer %s with y or n", label)
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("owner bootstrap failed", "error", err)
		os.Exit(1)
	}
}
