package tokens

import "testing"

func TestNewIsUniqueAndHashed(t *testing.T) {
	secret, hash, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) < 40 {
		t.Errorf("token too short (%d chars) — not 256 bits of entropy", len(secret))
	}
	if secret == hash {
		t.Error("stored hash equals the secret — a DB read would leak usable tokens")
	}
	if Hash(secret) != hash {
		t.Error("Hash does not reproduce the stored hash")
	}

	other, _, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if other == secret {
		t.Error("two tokens collided")
	}
}
