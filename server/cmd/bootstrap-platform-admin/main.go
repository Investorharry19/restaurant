package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"restaurant/server/internal/config"
	"restaurant/server/internal/database"
	"restaurant/server/internal/staffauth"
)

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	defer clear(password)

	reader := bufio.NewReader(os.Stdin)
	name, err := prompt(reader, "Administrator name")
	if err != nil {
		return err
	}
	email, err := prompt(reader, "Administrator email")
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if name == "" || len(name) > 120 {
		return fmt.Errorf("administrator name must be between 1 and 120 characters")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return fmt.Errorf("administrator email is invalid")
	}
	passwordHash, err := staffauth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
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

	if _, err := db.ExecContext(ctx, `
		INSERT INTO platform_admins (name, email, password_hash)
		VALUES ($1, $2, $3)
	`, name, email, passwordHash); err != nil {
		return fmt.Errorf("create platform administrator: %w", err)
	}

	fmt.Printf("Created platform administrator %q.\n", email)
	return nil
}

func readPassword() ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("run this command in an interactive terminal to enter the administrator password")
	}
	fmt.Print("Administrator password (12-1024 bytes): ")
	password, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("read administrator password: %w", err)
	}
	if len(bytes.TrimSpace(password)) == 0 {
		clear(password)
		return nil, fmt.Errorf("administrator password must not be blank")
	}
	if len(password) < 12 || len(password) > 1024 {
		clear(password)
		return nil, fmt.Errorf("administrator password must be between 12 and 1024 bytes")
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

func main() {
	if err := run(); err != nil {
		slog.Error("platform administrator bootstrap failed", "error", err)
		os.Exit(1)
	}
}
