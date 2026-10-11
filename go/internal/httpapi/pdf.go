package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
	"github.com/quests-inc/quests/go/internal/pdfdoc"
	"github.com/quests-inc/quests/go/internal/store"
)

// pdfSlots bounds concurrent renders: each is CPU- and memory-heavy.
var pdfSlots = make(chan struct{}, 3)

const pdfRenderTimeout = 60 * time.Second

func (s *Server) registerPDF(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/export/pdf", s.postRenderPDF)
}

type renderPDFRequest struct {
	Document pdfdoc.Document `json:"document"`
	Filename string          `json:"filename"`
}

// postRenderPDF turns a document (title, markdown, steps — see internal/pdfdoc) into a PDF. The renderer
// is in-process Go and never reads the network or the disk: the only things that reach the page besides
// the text are attachment images, fetched here under the caller's own workspace.
func (s *Server) postRenderPDF(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, pdfdoc.MaxMarkdownBytes+(1<<20))
	var req renderPDFRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON or document too large")
		return
	}
	if len(req.Document.Blocks) == 0 && strings.TrimSpace(req.Document.Title) == "" {
		writeErr(w, http.StatusBadRequest, "document is empty")
		return
	}

	select {
	case pdfSlots <- struct{}{}:
		defer func() { <-pdfSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		writeErr(w, http.StatusTooManyRequests, "рендер PDF занят, повторите через несколько секунд")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), pdfRenderTimeout)
	defer cancel()
	var out bytes.Buffer
	if err := pdfdoc.Render(ctx, req.Document, s.attachmentImageLoader(ctx), &out); err != nil {
		switch {
		case errors.Is(err, pdfdoc.ErrTooLong):
			writeErr(w, http.StatusRequestEntityTooLarge, "документ слишком длинный для PDF")
		case errors.Is(err, context.DeadlineExceeded):
			writeErr(w, http.StatusGatewayTimeout, "рендер PDF не уложился в отведённое время")
		default:
			writeErr(w, http.StatusInternalServerError, "pdf render failed: "+err.Error())
		}
		return
	}

	filename := req.Filename
	if filename == "" {
		filename = uuid.NewString() + ".pdf"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", knowledgehttp.ContentDisposition(sanitizeFilename(filename)))
	w.Header().Set("Content-Length", fmt.Sprint(out.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out.Bytes())
}

// attachmentImageLoader fetches attachment images for the renderer with the same rules as the inline
// image endpoint: the caller's workspace (row-level security), not hidden, and a raster type that the
// upload scan recorded — never the client's claim.
func (s *Server) attachmentImageLoader(ctx context.Context) pdfdoc.ImageLoader {
	return func(id int64) ([]byte, error) {
		if !s.knowledgeHTTP().WebDAVOK() {
			return nil, errors.New("attachment storage is not configured")
		}
		a, err := s.Store.Knowledge.GetAttachment(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errors.New("attachment not found")
			}
			return nil, err
		}
		if a.Hidden {
			return nil, errors.New("attachment not found")
		}
		ctype, _, _ := strings.Cut(a.ContentTypeDetected, ";")
		if !knowledgehttp.InlineImageTypes[strings.ToLower(strings.TrimSpace(ctype))] {
			return nil, errors.New("attachment is not a raster image")
		}
		body, err := s.WebDAV.Get(ctx, a.WebDAVPath)
		if err != nil {
			return nil, err
		}
		defer body.Close()
		data, err := io.ReadAll(io.LimitReader(body, pdfdoc.MaxImageBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > pdfdoc.MaxImageBytes {
			return nil, errors.New("image is too large")
		}
		return data, nil
	}
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
