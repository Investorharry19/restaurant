package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

func EncryptTicketToken(key []byte, token string) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("ticket token encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create ticket token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create ticket token authenticator: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate ticket token nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(token), []byte("restaurant-push-ticket-token-v1")), nil
}

func DecryptTicketToken(key, ciphertext []byte) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("ticket token encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create ticket token cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create ticket token authenticator: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return "", fmt.Errorf("ticket token ciphertext is truncated")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, ciphertext[gcm.NonceSize():], []byte("restaurant-push-ticket-token-v1"))
	if err != nil {
		return "", fmt.Errorf("authenticate ticket token ciphertext: %w", err)
	}
	return string(plaintext), nil
}
