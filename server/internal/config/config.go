package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type LookupEnv func(string) (string, bool)

type Config struct {
	AppEnv        string
	HTTPAddr      string
	DatabaseURL   string
	AppBaseURL    string
	FrontendURL   string
	CORSOrigins   []string
	MigrationsDir string
	TLSCertFile   string
	TLSKeyFile    string
}

func Load() (Config, error) {
	return Parse(os.LookupEnv)
}

func Parse(lookup LookupEnv) (Config, error) {
	appEnv, err := required(lookup, "APP_ENV")
	if err != nil {
		return Config{}, err
	}
	if appEnv != "development" && appEnv != "test" && appEnv != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, or production")
	}

	httpAddr, err := required(lookup, "HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}
	if err := validateHTTPAddr(httpAddr); err != nil {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR: %w", err)
	}

	databaseURL, err := required(lookup, "DATABASE_URL")
	if err != nil {
		return Config{}, err
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid DATABASE_URL: %w", err)
	}

	appBaseURL, err := required(lookup, "APP_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	if _, err := parseOrigin(appBaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid APP_BASE_URL: %w", err)
	}

	frontendURL, err := required(lookup, "FRONTEND_URL")
	if err != nil {
		return Config{}, err
	}
	frontendOrigin, err := parseOrigin(frontendURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid FRONTEND_URL: %w", err)
	}

	originsValue, err := required(lookup, "CORS_ORIGINS")
	if err != nil {
		return Config{}, err
	}
	origins, err := parseOrigins(originsValue)
	if err != nil {
		return Config{}, fmt.Errorf("invalid CORS_ORIGINS: %w", err)
	}
	if !contains(origins, frontendOrigin) {
		return Config{}, fmt.Errorf("CORS_ORIGINS must include the FRONTEND_URL origin")
	}

	migrationsDir := optional(lookup, "MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	certFile := optional(lookup, "TLS_CERT_FILE")
	keyFile := optional(lookup, "TLS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		return Config{}, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	if appEnv == "production" && certFile == "" {
		return Config{}, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE are required in production")
	}

	return Config{
		AppEnv:        appEnv,
		HTTPAddr:      httpAddr,
		DatabaseURL:   databaseURL,
		AppBaseURL:    appBaseURL,
		FrontendURL:   frontendOrigin,
		CORSOrigins:   origins,
		MigrationsDir: migrationsDir,
		TLSCertFile:   certFile,
		TLSKeyFile:    keyFile,
	}, nil
}

func required(lookup LookupEnv, key string) (string, error) {
	value, ok := lookup(key)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optional(lookup LookupEnv, key string) string {
	value, _ := lookup(key)
	return strings.TrimSpace(value)
}

func validateHTTPAddr(value string) error {
	_, portValue, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("must be a host:port address: %w", err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("must be a valid PostgreSQL URL")
	}
	if (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return fmt.Errorf("must use postgres:// or postgresql:// and include a host")
	}
	return nil
}

func parseOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("must be a valid HTTP(S) origin")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" ||
		parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return "", fmt.Errorf("must be an HTTP(S) origin without credentials, path, query, or fragment")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func parseOrigins(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("origin list contains an empty value")
		}
		if part == "*" {
			return nil, fmt.Errorf("wildcard origins are not allowed")
		}
		origin, err := parseOrigin(part)
		if err != nil {
			return nil, err
		}
		if !contains(origins, origin) {
			origins = append(origins, origin)
		}
	}
	return origins, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
