package config

import "testing"

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
