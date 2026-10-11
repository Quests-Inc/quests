// Package secretbox encrypts small secrets (an API key) for storage in the database.
//
// AES-256-GCM with a key that never lives in the database: QUESTS_SECRET_KEY
// (32 bytes, base64) when set, else a random key generated once into
// <data dir>/secret.key (0600). A database dump on its own therefore reveals
// nothing; losing the key loses the secrets (they are re-entered, not recovered).
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const prefix = "v1:"

// ErrBadKey is returned when the stored value cannot be opened with the current key.
var ErrBadKey = errors.New("секрет нельзя расшифровать текущим ключом (QUESTS_SECRET_KEY / data/secret.key изменился?)")

// Box seals and opens strings.
type Box struct{ aead cipher.AEAD }

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secretbox: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Load returns the Box for this installation (see the package comment).
func Load(dataDir string) (*Box, error) {
	if raw := strings.TrimSpace(os.Getenv("QUESTS_SECRET_KEY")); raw != "" {
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("QUESTS_SECRET_KEY is not base64: %w", err)
		}
		return New(key)
	}
	path := filepath.Join(dataDir, "secret.key")
	if raw, err := os.ReadFile(path); err == nil {
		key, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if derr != nil {
			return nil, fmt.Errorf("%s is not base64: %w", path, derr)
		}
		return New(key)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return New(key)
}

// Seal encrypts s; the result is safe to store as text.
func (b *Box) Seal(s string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(s), nil)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

// Open reverses Seal.
func (b *Box) Open(sealed string) (string, error) {
	raw, ok := strings.CutPrefix(sealed, prefix)
	if !ok {
		return "", ErrBadKey
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < b.aead.NonceSize() {
		return "", ErrBadKey
	}
	plain, err := b.aead.Open(nil, data[:b.aead.NonceSize()], data[b.aead.NonceSize():], nil)
	if err != nil {
		return "", ErrBadKey
	}
	return string(plain), nil
}
