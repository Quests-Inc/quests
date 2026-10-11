// Package identity holds password hashing, opaque token generation and the
// user/session/token store. Data in Quests is shared — this gates access to
// the instance, it does not scope rows to an owner.
package identity

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// DefaultIterations for PBKDF2-HMAC-SHA256. Deliberately high: logins are rare
// and a stolen quests.db should stay expensive to crack offline.
const DefaultIterations = 600_000

const saltLen = 16
const keyLen = 32

// HashPassword returns a Django-style encoded hash:
// pbkdf2_sha256$<iters>$<salt_b64>$<key_b64>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hashWith(password, salt, DefaultIterations)
}

func hashWith(password string, salt []byte, iters int) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iters, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iters,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the encoded hash. It runs in
// time independent of where the mismatch is, and never distinguishes a
// malformed hash from a wrong password to the caller.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iters, err := strconv.Atoi(parts[1])
	if err != nil || iters <= 0 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iters, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
