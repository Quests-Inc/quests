package identity

import (
	"strings"
	"testing"
)

// Iterations are fixed at DefaultIterations in production; tests reuse the
// exported helpers so the encoding stays the contract under test.

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2_sha256$") {
		t.Fatalf("unexpected encoding: %s", hash)
	}
	if !VerifyPassword("correct-horse-battery", hash) {
		t.Error("correct password rejected")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Error("wrong password accepted")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	a, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same-password")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical — salt is not random")
	}
	if !VerifyPassword("same-password", a) || !VerifyPassword("same-password", b) {
		t.Error("salted hashes must both verify")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"no prefix":      "deadbeef",
		"wrong algo":     "bcrypt$10$salt$hash",
		"missing field":  "pbkdf2_sha256$600000$c2FsdA==",
		"bad iterations": "pbkdf2_sha256$abc$c2FsdA==$aGFzaA==",
		"zero iters":     "pbkdf2_sha256$0$c2FsdA==$aGFzaA==",
		"bad base64":     "pbkdf2_sha256$600000$!!!$!!!",
	}
	for name, encoded := range cases {
		if VerifyPassword("anything", encoded) {
			t.Errorf("%s: malformed hash accepted", name)
		}
	}
}
