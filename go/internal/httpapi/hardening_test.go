package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valirum/quests/go/internal/store"
)

func TestSecureHeadersAndBodyCap(t *testing.T) {
	s := &Server{}
	var read int64
	h := s.secure(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1<<20)
		for {
			n, err := r.Body.Read(b)
			read += int64(n)
			if err != nil {
				break
			}
		}
	}))
	big := strings.NewReader(strings.Repeat("a", maxJSONBody+1024))
	req := httptest.NewRequest(http.MethodPost, "/api/quests", big)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if read > maxJSONBody {
		t.Fatalf("JSON body read %d bytes, cap is %d", read, maxJSONBody)
	}
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("weak CSP: %q", csp)
	}
}

func TestIconsAreSandboxed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "1.svg")
	if err := os.WriteFile(p, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serveIcon(rec, httptest.NewRequest(http.MethodGet, "/api/questlines/1/icon", nil), p)
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Fatalf("SVG icon served without a sandbox CSP: %q", csp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("icon served without nosniff")
	}
}

func TestReservedSecretNames(t *testing.T) {
	for _, bad := range []string{"PATH", "path", "LD_PRELOAD", "BASH_ENV", "SHELLOPTS", "PS4", "QUESTS_WORKSPACE", "PYTHONSTARTUP", "NODE_OPTIONS", "GIT_SSH_COMMAND"} {
		if !store.ReservedSecretName(bad) {
			t.Errorf("%s should be reserved", bad)
		}
	}
	for _, ok := range []string{"API_KEY", "GROQ_TOKEN", "DB_PASSWORD", "REFCOOL_URL"} {
		if store.ReservedSecretName(ok) {
			t.Errorf("%s should be allowed", ok)
		}
	}
}
