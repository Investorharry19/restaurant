package staffauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeySize uint32 = 32
)

func HashPassword(password []byte) (string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", fmt.Errorf("password must be between 12 and 1024 bytes")
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, argonKeySize)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(encoded string, password []byte) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}

	parameters := make(map[string]uint32, 3)
	for _, parameter := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(parameter, "=", 2)
		if len(keyValue) != 2 {
			return false
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil {
			return false
		}
		parameters[keyValue[0]] = uint32(value)
	}
	if parameters["m"] != argonMemory ||
		parameters["t"] != argonTime ||
		parameters["p"] != uint32(argonThreads) ||
		len(parameters) != 3 {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != int(argonKeySize) {
		return false
	}
	actual := argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, argonKeySize)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func DummyPasswordHash() string {
	const salt = "restaurant-queue-dummy"
	hash := argon2.IDKey(
		[]byte("not-a-real-account-password"),
		[]byte(salt),
		argonTime,
		argonMemory,
		argonThreads,
		argonKeySize,
	)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString([]byte(salt)),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}
