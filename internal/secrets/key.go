package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadMasterKey читает 32 байта из NETLYNX_SECRETS_KEY (base64) или файла NETLYNX_SECRETS_KEY_FILE.
// Пустые оба → (nil, nil) — режим compat без шифрования.
func LoadMasterKey(envKey, keyFile string) ([]byte, error) {
	envKey = strings.TrimSpace(envKey)
	keyFile = strings.TrimSpace(keyFile)
	if keyFile != "" {
		b, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("NETLYNX_SECRETS_KEY_FILE: %w", err)
		}
		raw := strings.TrimSpace(string(b))
		return decodeKey(raw)
	}
	if envKey == "" {
		return nil, nil
	}
	return decodeKey(envKey)
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: not valid base64", ErrBadKey)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("%w: need %d bytes after base64 decode, got %d", ErrBadKey, keySize, len(key))
	}
	return key, nil
}

// GenerateMasterKeyBase64 — новый случайный ключ (подсказка для админа).
func GenerateMasterKeyBase64() (string, error) {
	b := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
