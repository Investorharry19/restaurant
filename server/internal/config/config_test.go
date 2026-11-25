package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRequiresExplicitConfiguration(t *testing.T) {
	_, err := Parse(func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("expected missing environment variables to fail")
	}
}

func TestParseValidatesOriginAllowlist(t *testing.T) {
	values := validEnvironment()
	values["CORS_ORIGINS"] = "*"

	_, err := Parse(lookup(values))
	if err == nil {
		t.Fatal("expected wildcard CORS origin to fail")
	}
}

func TestParseRequiresTLSInProduction(t *testing.T) {
	values := validEnvironment()
	values["APP_ENV"] = "production"

	_, err := Parse(lookup(values))
	if err == nil {
		t.Fatal("expected production configuration without TLS to fail")
	}
}

func TestParseAcceptsValidDevelopmentConfiguration(t *testing.T) {
	config, err := Parse(lookup(validEnvironment()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want %q", config.HTTPAddr, ":8080")
	}
	if config.MigrationsDir != "migrations" {
		t.Fatalf("MigrationsDir = %q, want %q", config.MigrationsDir, "migrations")
	}
	if config.SessionTTL != 24*time.Hour {
		t.Fatalf("SessionTTL = %s, want 24h", config.SessionTTL)
	}
}

func TestParseValidatesStaffSessionSettings(t *testing.T) {
	values := validEnvironment()
	values["SESSION_TTL_HOURS"] = "48"
	values["SESSION_COOKIE_DOMAIN"] = ".restaurant.example"

	config, err := Parse(lookup(values))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.SessionTTL != 48*time.Hour {
		t.Fatalf("SessionTTL = %s, want 48h", config.SessionTTL)
	}
	if config.SessionCookieDomain != ".restaurant.example" {
		t.Fatalf("SessionCookieDomain = %q", config.SessionCookieDomain)
	}

	for _, invalid := range []map[string]string{
		{"SESSION_TTL_HOURS": "0"},
		{"SESSION_TTL_HOURS": "721"},
		{"SESSION_TTL_HOURS": "not-a-number"},
		{"SESSION_COOKIE_DOMAIN": "https://restaurant.example"},
		{"SESSION_COOKIE_DOMAIN": "restaurant.example/path"},
	} {
		values := validEnvironment()
		for key, value := range invalid {
			values[key] = value
		}
		if _, err := Parse(lookup(values)); err == nil {
			t.Fatalf("Parse() accepted invalid session settings %v", invalid)
		}
	}
}

func TestParseValidatesResendSettings(t *testing.T) {
	values := validEnvironment()
	values["RESEND_API_KEY"] = "re_test_key"
	values["RESEND_FROM_EMAIL"] = "Restaurant Queue <onboarding@example.com>"
	config, err := Parse(lookup(values))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.ResendAPIKey != "re_test_key" ||
		config.ResendFromEmail != "Restaurant Queue <onboarding@example.com>" {
		t.Fatalf("Resend settings were not loaded: %#v", config)
	}

	for _, invalid := range []map[string]string{
		{"RESEND_API_KEY": "re_test_key"},
		{"RESEND_API_KEY": "re_test_key", "RESEND_FROM_EMAIL": "not-an-email"},
	} {
		values := validEnvironment()
		for key, value := range invalid {
			values[key] = value
		}
		if _, err := Parse(lookup(values)); err == nil {
			t.Fatalf("Parse() accepted invalid Resend settings %v", invalid)
		}
	}

	productionValues := validEnvironment()
	productionValues["APP_ENV"] = "production"
	productionValues["TLS_CERT_FILE"] = "server.crt"
	productionValues["TLS_KEY_FILE"] = "server.key"
	if _, err := Parse(lookup(productionValues)); err == nil {
		t.Fatal("Parse() accepted production configuration without Resend settings")
	}
}

func TestLoadEnvironmentReadsRepositoryEnvAndPrefersProcessValues(t *testing.T) {
	repositoryRoot := t.TempDir()
	serverDir := filepath.Join(repositoryRoot, "server")
	commandDir := filepath.Join(serverDir, "cmd", "api")
	if err := os.MkdirAll(commandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "go.mod"), []byte("module example/server\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	envFile := strings.Join([]string{
		"APP_ENV=development",
		"HTTP_ADDR=:8080",
		"DATABASE_URL=postgres://localhost:5432/restaurant?sslmode=disable",
		"APP_BASE_URL=http://localhost:3000",
		"FRONTEND_URL=http://localhost:3000",
		"CORS_ORIGINS=http://localhost:3000",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repositoryRoot, ".env"), []byte(envFile), 0o600); err != nil {
		t.Fatal(err)
	}

	processLookup := func(key string) (string, bool) {
		if key == "HTTP_ADDR" {
			return ":9090", true
		}
		return "", false
	}
	environment, err := loadEnvironment(commandDir, processLookup)
	if err != nil {
		t.Fatalf("loadEnvironment() error = %v", err)
	}

	config, err := Parse(environment)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want process value %q", config.HTTPAddr, ":9090")
	}
	if config.DatabaseURL != "postgres://localhost:5432/restaurant?sslmode=disable" {
		t.Fatalf("DatabaseURL did not come from .env")
	}
}

func TestLoadEnvironmentRejectsMalformedAssignments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_ENV\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadEnvironment(root, func(string) (string, bool) { return "", false })
	if err == nil || !strings.Contains(err.Error(), ":1: expected KEY=VALUE") {
		t.Fatalf("loadEnvironment() error = %v, want malformed assignment error", err)
	}
}

func validEnvironment() map[string]string {
	return map[string]string{
		"APP_ENV":        "development",
		"HTTP_ADDR":      ":8080",
		"DATABASE_URL":   "postgres://restaurant:restaurant@localhost:5432/restaurant?sslmode=disable",
		"APP_BASE_URL":   "http://localhost:3000",
		"FRONTEND_URL":   "http://localhost:3000",
		"CORS_ORIGINS":   "http://localhost:3000",
		"MIGRATIONS_DIR": "",
	}
}

func lookup(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
