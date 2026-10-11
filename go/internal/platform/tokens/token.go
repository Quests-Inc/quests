// Package tokens makes opaque secrets (session cookies, API tokens, invitation links, runner credentials)
// and the hashes that are stored instead of them.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// New returns a fresh opaque secret (32 random bytes, base64url) together
// with the hash stored in the DB. The secret is shown to the user once.
func New() (secret string, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	secret = base64.RawURLEncoding.EncodeToString(raw)
	return secret, Hash(secret), nil
}

// Hash is a plain SHA-256 of the secret. Unlike a password, the secret is
// already 256 bits of entropy, so there is nothing for a slow KDF to protect.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
