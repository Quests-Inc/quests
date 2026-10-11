package secretbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndTamper(t *testing.T) {
	t.Setenv("QUESTS_SECRET_KEY", "")
	dir := t.TempDir()
	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal("gsk_secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "gsk_secret-value" || len(sealed) < 20 {
		t.Fatalf("not encrypted: %q", sealed)
	}
	if got, err := b.Open(sealed); err != nil || got != "gsk_secret-value" {
		t.Fatalf("round trip: %q %v", got, err)
	}
	// Same plaintext seals differently (random nonce).
	again, _ := b.Seal("gsk_secret-value")
	if again == sealed {
		t.Fatal("nonce reused")
	}
	// A second Load reads the same key file.
	b2, _ := Load(dir)
	if got, err := b2.Open(sealed); err != nil || got != "gsk_secret-value" {
		t.Fatalf("key file not reused: %q %v", got, err)
	}
	if st, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode: %v %v", st, err)
	}
	// Tampering or a different key fails closed.
	if _, err := b.Open(sealed[:len(sealed)-4] + "AAAA"); err == nil {
		t.Fatal("tampered value opened")
	}
	other, _ := Load(t.TempDir())
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("opened with another key")
	}
}
