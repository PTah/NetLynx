package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	prefixV1  = "enc:v1:"
	keySize   = 32
	nonceSize = 12 // GCM standard
)

var (
	ErrNoKey       = errors.New("secrets: master key not configured")
	ErrBadKey      = errors.New("secrets: invalid master key")
	ErrBadCipher   = errors.New("secrets: invalid ciphertext")
	ErrDecryptFail = errors.New("secrets: decrypt failed")
)

// Box — AES-256-GCM envelope для at-rest секретов в TEXT-колонках.
type Box struct {
	gcm cipher.AEAD
}

// NewBox создаёт Box из 32-байтного ключа.
func NewBox(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("%w: need %d bytes, got %d", ErrBadKey, keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

// Enabled — true, если шифрование при записи активно.
func (b *Box) Enabled() bool {
	return b != nil && b.gcm != nil
}

// IsEncrypted — значение уже в формате enc:v1:…
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, prefixV1)
}

// Seal шифрует plaintext. Пустая строка остаётся пустой. Без ключа — ErrNoKey.
func (b *Box) Seal(plain string) (string, error) {
	if !b.Enabled() {
		return "", ErrNoKey
	}
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := b.gcm.Seal(nil, nonce, []byte(plain), nil)
	raw := append(nonce, ct...)
	return prefixV1 + base64.RawStdEncoding.EncodeToString(raw), nil
}

// Open расшифровывает enc:v1:… или возвращает legacy plaintext как есть.
// Без ключа: enc:v1 → ошибка; plaintext → как есть.
func (b *Box) Open(stored string) (string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return "", nil
	}
	if !IsEncrypted(stored) {
		return stored, nil
	}
	if !b.Enabled() {
		return "", ErrNoKey
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, prefixV1))
	if err != nil || len(payload) < nonceSize+b.gcm.Overhead() {
		return "", ErrBadCipher
	}
	nonce, ct := payload[:nonceSize], payload[nonceSize:]
	pt, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecryptFail
	}
	return string(pt), nil
}

// SealPtr — nil остаётся nil; иначе Seal(*p).
func (b *Box) SealPtr(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	if !b.Enabled() {
		return p, nil
	}
	s, err := b.Seal(*p)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// OpenPtr — nil остаётся nil; иначе Open(*p).
func (b *Box) OpenPtr(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	s, err := b.Open(*p)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// MaybeSeal — если Box nil/disabled, возвращает plaintext без изменений.
func MaybeSeal(b *Box, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if !b.Enabled() {
		return plain, nil
	}
	return b.Seal(plain)
}

// MaybeSealPtr — если Box disabled, указатель как есть.
func MaybeSealPtr(b *Box, p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	if !b.Enabled() {
		return p, nil
	}
	return b.SealPtr(p)
}

// MustOpenPtr — OpenPtr; при ошибке возвращает исходный указатель и error (caller логирует).
func MustOpenPtr(b *Box, p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	if b == nil || !b.Enabled() {
		if IsEncrypted(*p) {
			return nil, ErrNoKey
		}
		return p, nil
	}
	return b.OpenPtr(p)
}
