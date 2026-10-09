package httpapi

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxPDFHTMLBytes caps the HTML payload accepted for rendering — generous
// for a note/quest export, small enough to bound weasyprint's memory/time.
const maxPDFHTMLBytes = 20 << 20 // 20MB

// pdfRenderScript renders with every external fetch refused (see render.py): the HTML
// is user-supplied, and WeasyPrint would otherwise embed local files and internal
// URLs it finds in it into the PDF it hands back.
//
//go:embed pdfrender/render.py
var pdfRenderScript []byte

// pdfSlots bounds concurrent renders: each is a CPU- and memory-heavy process.
var pdfSlots = make(chan struct{}, 2)

// pdfPython is the interpreter that has WeasyPrint: QUESTS_PDF_PYTHON, else the
// project venv, else python3 on PATH (the container puts the venv first).
func (s *Server) pdfPython() string {
	if v := strings.TrimSpace(os.Getenv("QUESTS_PDF_PYTHON")); v != "" {
		return v
	}
	if s.Root != "" {
		if p := filepath.Join(s.Root, ".venv", "bin", "python"); fileExists(p) {
			return p
		}
	}
	return "python3"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (s *Server) registerPDF(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/export/pdf", s.postRenderPDF)
}

type renderPDFRequest struct {
	HTML     string `json:"html"`
	Filename string `json:"filename"`
}

// postRenderPDF converts a self-contained HTML document (inline CSS, no
// external assets) to PDF via WeasyPrint — a real HTML+CSS layout/paginator,
// not a screenshot: text stays selectable, @page/break-inside rules drive
// margins and pagination instead of hand-rolled pixel math.
func (s *Server) postRenderPDF(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPDFHTMLBytes)
	var req renderPDFRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON or HTML too large")
		return
	}
	if req.HTML == "" {
		writeErr(w, http.StatusBadRequest, "html is required")
		return
	}

	workDir, err := os.MkdirTemp("", "quests-pdf-*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(workDir)

	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusTooManyRequests, "рендер PDF занят, повторите через несколько секунд")
		return
	}

	inPath := filepath.Join(workDir, "in.html")
	outPath := filepath.Join(workDir, "out.pdf")
	scriptPath := filepath.Join(workDir, "render.py")
	if err := os.WriteFile(scriptPath, pdfRenderScript, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(inPath, []byte(req.HTML), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pdfPython(), scriptPath, inPath, outPath)
	// The renderer needs nothing from the server's environment (database URL, tokens…).
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + workDir, "XDG_CACHE_HOME=" + workDir, "LANG=C.UTF-8"}
	cmd.Dir = workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		writeErr(w, http.StatusInternalServerError, "pdf render failed: "+msg)
		return
	}

	pdf, err := os.ReadFile(outPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := req.Filename
	if filename == "" {
		filename = uuid.NewString() + ".pdf"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeFilename(filename)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func sanitizeFilename(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "export.pdf"
	}
	return string(out)
}
