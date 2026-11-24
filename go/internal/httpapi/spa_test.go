package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFWorkerSurvivesSPARebuild(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "frontend", "dist", "assets")
	if err := os.MkdirAll(assets, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "pdf.worker.min.mjs"), []byte("export const worker = true"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Root: root}
	mux := http.NewServeMux()
	s.mountSPA(mux)
	for _, url := range []string{"/assets/pdf.worker.min.mjs", "/assets/pdf.worker.min-CIqJB0yW.mjs"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "worker = true") || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("worker %s: %d %v %s", url, w.Code, w.Header(), w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/assets/missing.mjs", nil))
	if w.Code != 404 {
		t.Fatalf("unrelated missing assets must remain 404: %d", w.Code)
	}
}
