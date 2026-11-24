package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pdfWorkerAsset = regexp.MustCompile(`^/assets/pdf\.worker\.min(?:-[A-Za-z0-9_-]+)?\.mjs$`)

func (s *Server) mountSPA(mux *http.ServeMux) {
	dist := filepath.Join(s.Root, "frontend", "dist")
	if st, err := os.Stat(dist); err != nil || !st.IsDir() {
		return
	}
	assets := filepath.Join(dist, "assets")
	if st, err := os.Stat(assets); err == nil && st.IsDir() {
		files := http.StripPrefix("/assets/", http.FileServer(http.Dir(assets)))
		mux.Handle("GET /assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pdfWorkerAsset.MatchString(r.URL.Path) {
				// Keep existing versioned workers when present; stale tabs can use the
				// stable worker after a build removes the old hashed filename.
				target := filepath.Join(assets, filepath.Base(r.URL.Path))
				if _, err := os.Stat(target); os.IsNotExist(err) {
					target = filepath.Join(assets, "pdf.worker.min.mjs")
				}
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
				http.ServeFile(w, r, target)
				return
			}
			files.ServeHTTP(w, r)
		}))
	}
	index := filepath.Join(dist, "index.html")
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, index)
	})
	mux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) {
		p := r.PathValue("path")
		if p == "" || strings.HasPrefix(p, "api/") || p == "api" || p == "ws" ||
			p == "docs" || p == "openapi.json" || p == "redoc" {
			http.NotFound(w, r)
			return
		}
		candidate := filepath.Join(dist, filepath.Clean("/"+p))
		if !strings.HasPrefix(candidate, dist) {
			http.NotFound(w, r)
			return
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		}
		http.ServeFile(w, r, index)
	})
}
