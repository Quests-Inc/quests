package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValuesCLIUsesOneTypedPackage(t *testing.T) {
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/api/notes/7/values" || !strings.Contains(string(raw), `"value":false`) || !strings.Contains(string(raw), `"formula":"later+1"`) {
			t.Errorf("request: %s %s %s", r.Method, r.URL.Path, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(file, []byte(`[{"name":"flag","type":"bool","value":false},{"name":"n","type":"int","formula":"later+1"},{"name":"later","type":"int","value":0}]`), 0600); err != nil {
		t.Fatal(err)
	}
	code, err := cmdValues(NewClient(server.URL, false), []string{"set", "note=7", file})
	if code != 0 || err != nil || count != 1 {
		t.Fatal(code, err, count)
	}
	if _, err := cmdValues(NewClient(server.URL, false), []string{"set", "note=oops", file}); err == nil {
		t.Fatal("invalid owner accepted")
	}
}
