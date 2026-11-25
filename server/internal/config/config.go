package config

import (
	"bufio"
	"bytes"
	"crypto/elliptic"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type LookupEnv func(string) (string, bool)

type Config struct {
	AppEnv              string
	HTTPAddr            string
	DatabaseURL         string
	AppBaseURL          string
	FrontendURL         string
	CORSOrigins         []string
	MigrationsDir       string
	ResendAPIKey        string
	ResendFromEmail     string
	VAPIDPublicKey      string
	VAPIDPrivateKey     string
	VAPIDSubject        string
	PushEncryptionKey   []byte
	SessionTTL          time.Duration
	SessionCookieDomain string
	TLSCertFile         string
	TLSKeyFile          string
}

func Load() (Config, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("get working directory: %w", err)
	}

	lookup, err := loadEnvironment(workingDir, os.LookupEnv)
	if err != nil {
		return Config{}, fmt.Errorf("load environment: %w", err)
	}

	return Parse(lookup)
}

func loadEnvironment(startDir string, processLookup LookupEnv) (LookupEnv, error) {
	path, found, err := findEnvFile(startDir)
	if err != nil {
		return nil, err
	}
	if !found {
		return processLookup, nil
	}

	fileValues, err := parseEnvFile(path)
	if err != nil {
		return nil, err
	}

	return func(key string) (string, bool) {
		if value, ok := processLookup(key); ok {
			return value, true
		}
		value, ok := fileValues[key]
		return value, ok
	}, nil
}

func findEnvFile(startDir string) (string, bool, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, fmt.Errorf("resolve working directory: %w", err)
	}

	for {
		envPath := filepath.Join(dir, ".env")
		found, err := regularFileExists(envPath)
		if err != nil {
			return "", false, err
		}
		if found {
			return envPath, true, nil
		}

		modulePath := filepath.Join(dir, "go.mod")
		found, err = regularFileExists(modulePath)
		if err != nil {
			return "", false, err
		}
		if found {
			envPath = filepath.Join(filepath.Dir(dir), ".env")
			found, err = regularFileExists(envPath)
			if err != nil {
				return "", false, err
			}
			return envPath, found, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	return true, nil
}

func parseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		assignment := strings.SplitN(line, "=", 2)
		if len(assignment) != 2 {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNumber)
		}

		key := strings.TrimSpace(assignment[0])
		if !validEnvKey(key) {
			return nil, fmt.Errorf("%s:%d: invalid environment variable name", path, lineNumber)
		}

		value := strings.TrimSpace(assignment[1])
		if len(value) > 0 && (value[0] == '"' || value[0] == '\'') {
			quote := value[0]
			if len(value) < 2 || value[len(value)-1] != quote {
				return nil, fmt.Errorf("%s:%d: unterminated quoted value", path, lineNumber)
			}
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return values, nil
}

func validEnvKey(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' ||
			(char >= 'A' && char <= 'Z') ||
			(char >= 'a' && char <= 'z') ||
			(index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
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

	sessionTTLHours := 24
	if value := optional(lookup, "SESSION_TTL_HOURS"); value != "" {
		sessionTTLHours, err = strconv.Atoi(value)
		if err != nil || sessionTTLHours < 1 || sessionTTLHours > 720 {
			return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be between 1 and 720")
		}
	}
	sessionCookieDomain := optional(lookup, "SESSION_COOKIE_DOMAIN")
	if sessionCookieDomain != "" {
		if err := validateCookieDomain(sessionCookieDomain); err != nil {
			return Config{}, fmt.Errorf("invalid SESSION_COOKIE_DOMAIN: %w", err)
		}
	}

	certFile := optional(lookup, "TLS_CERT_FILE")
	keyFile := optional(lookup, "TLS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		return Config{}, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	if appEnv == "production" && certFile == "" {
		return Config{}, fmt.Errorf("TLS_CERT_FILE and TLS_KEY_FILE are required in production")
	}

	resendAPIKey := optional(lookup, "RESEND_API_KEY")
	resendFromEmail := optional(lookup, "RESEND_FROM_EMAIL")
	if appEnv == "production" && (resendAPIKey == "" || resendFromEmail == "") {
		return Config{}, fmt.Errorf("RESEND_API_KEY and RESEND_FROM_EMAIL are required in production")
	}
	if (resendAPIKey == "") != (resendFromEmail == "") {
		return Config{}, fmt.Errorf("RESEND_API_KEY and RESEND_FROM_EMAIL must be set together")
	}
	if resendFromEmail != "" {
		if _, err := mail.ParseAddress(resendFromEmail); err != nil {
			return Config{}, fmt.Errorf("RESEND_FROM_EMAIL must be a valid email address")
		}
	}

	vapidPublicKey := optional(lookup, "VAPID_PUBLIC_KEY")
	vapidPrivateKey := optional(lookup, "VAPID_PRIVATE_KEY")
	vapidSubject := optional(lookup, "VAPID_SUBJECT")
	pushEncryptionKeyValue := optional(lookup, "PUSH_TOKEN_ENCRYPTION_KEY")
	pushSettingsPresent := vapidPublicKey != "" ||
		vapidPrivateKey != "" ||
		vapidSubject != "" ||
		pushEncryptionKeyValue != ""
	var pushEncryptionKey []byte
	if pushSettingsPresent {
		if vapidPublicKey == "" || vapidPrivateKey == "" || vapidSubject == "" || pushEncryptionKeyValue == "" {
			return Config{}, fmt.Errorf("VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY, VAPID_SUBJECT, and PUSH_TOKEN_ENCRYPTION_KEY must be configured together")
		}
		if err := validateVAPIDKeyPair(vapidPublicKey, vapidPrivateKey); err != nil {
			return Config{}, fmt.Errorf("invalid VAPID key pair: %w", err)
		}
		if err := validateVAPIDSubject(vapidSubject); err != nil {
			return Config{}, fmt.Errorf("invalid VAPID_SUBJECT: %w", err)
		}
		pushEncryptionKey, err = decodePushEncryptionKey(pushEncryptionKeyValue)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PUSH_TOKEN_ENCRYPTION_KEY: %w", err)
		}
	}

	return Config{
		AppEnv:              appEnv,
		HTTPAddr:            httpAddr,
		DatabaseURL:         databaseURL,
		AppBaseURL:          appBaseURL,
		FrontendURL:         frontendOrigin,
		CORSOrigins:         origins,
		MigrationsDir:       migrationsDir,
		ResendAPIKey:        resendAPIKey,
		ResendFromEmail:     resendFromEmail,
		VAPIDPublicKey:      vapidPublicKey,
		VAPIDPrivateKey:     vapidPrivateKey,
		VAPIDSubject:        vapidSubject,
		PushEncryptionKey:   pushEncryptionKey,
		SessionTTL:          time.Duration(sessionTTLHours) * time.Hour,
		SessionCookieDomain: sessionCookieDomain,
		TLSCertFile:         certFile,
		TLSKeyFile:          keyFile,
	}, nil
}

func validateVAPIDKeyPair(publicKey, privateKey string) error {
	publicBytes, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil || len(publicBytes) != 65 || publicBytes[0] != 4 {
		return fmt.Errorf("public key must be an unpadded base64url P-256 point")
	}
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil || len(privateBytes) != 32 {
		return fmt.Errorf("private key must be an unpadded base64url 32-byte scalar")
	}
	curve := elliptic.P256()
	privateScalar := new(big.Int).SetBytes(privateBytes)
	if privateScalar.Sign() <= 0 || privateScalar.Cmp(curve.Params().N) >= 0 {
		return fmt.Errorf("private key scalar is outside the P-256 range")
	}
	x, y := curve.ScalarBaseMult(privateBytes)
	if x == nil || y == nil || !bytes.Equal(elliptic.Marshal(curve, x, y), publicBytes) {
		return fmt.Errorf("public and private keys do not match")
	}
	return nil
}

func validateVAPIDSubject(subject string) error {
	if strings.HasPrefix(subject, "mailto:") {
		address := strings.TrimPrefix(subject, "mailto:")
		if _, err := mail.ParseAddress(address); err == nil {
			return nil
		}
		return fmt.Errorf("must contain a valid mailto address or HTTPS URL")
	}
	parsed, err := url.Parse(subject)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("must contain a valid mailto address or HTTPS URL")
	}
	return nil
}

func decodePushEncryptionKey(value string) ([]byte, error) {
	decoders := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, decoder := range decoders {
		key, err := decoder.DecodeString(value)
		if err == nil && len(key) == 32 {
			return key, nil
		}
	}
	return nil, fmt.Errorf("must be a base64-encoded 32-byte key")
}

func validateCookieDomain(value string) error {
	if len(value) > 253 || strings.ContainsAny(value, "/?#@ :") {
		return fmt.Errorf("must be a hostname without scheme, path, port, or credentials")
	}
	domain := strings.TrimPrefix(value, ".")
	if domain == "" {
		return fmt.Errorf("must be a hostname")
	}
	if net.ParseIP(domain) != nil {
		return nil
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("must contain valid hostname labels")
		}
		for _, character := range label {
			if !(character >= 'A' && character <= 'Z') &&
				!(character >= 'a' && character <= 'z') &&
				!(character >= '0' && character <= '9') &&
				character != '-' {
				return fmt.Errorf("must contain valid hostname labels")
			}
		}
	}
	return nil
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
