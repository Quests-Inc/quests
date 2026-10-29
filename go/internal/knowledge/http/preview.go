package knowledgehttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/filepreview"
)

var previewSlots = make(chan struct{}, 2)

// Text-only previews use the same workspace/private-note visibility as downloads.
func (h *Handler) getAttachmentPreview(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	a, err := h.Store.GetAttachment(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	path, name, mime, size, scan := a.WebDAVPath, a.Filename, a.ContentTypeDetected, a.SizeBytes, a.ScanStatus
	revision, uploaded := a.CurrentRevision, a.UploadedAt
	if raw := r.URL.Query().Get("revision"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 {
			writeErr(w, 422, "invalid revision")
			return
		}
		rev, e := h.Store.GetAttachmentRevision(r.Context(), a.ID, n)
		if errors.Is(e, knowledge.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if e != nil {
			writeErr(w, 500, e.Error())
			return
		}
		path, name, mime, size, scan = rev.WebDAVPath, rev.Filename, rev.ContentTypeDetected, rev.SizeBytes, rev.ScanStatus
		revision, uploaded = rev.Revision, rev.UploadedAt
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	kind := filepreview.Kind(name, mime)
	empty := func() {
		w.Header().Del("ETag")
		writeJSON(w, 200, map[string]string{"kind": kind, "text": ""})
	}
	if kind == "" || scan != "clean" || kind != "text" && size > filepreview.MaxFile || !h.WebDAVOK() {
		empty()
		return
	}
	etag := fmt.Sprintf(`"preview-1-%d-%d-%d-%d"`, id, revision, size, uploaded.UnixNano())
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	select {
	case previewSlots <- struct{}{}:
		defer func() { <-previewSlots }()
	case <-ctx.Done():
		empty()
		return
	}
	body, e := h.WebDAV.Get(ctx, path)
	if e != nil {
		empty()
		return
	}
	defer body.Close()
	value, e := filepreview.Extract(ctx, kind, body)
	if e != nil || strings.TrimSpace(value) == "" {
		empty()
		return
	}
	writeJSON(w, 200, map[string]string{"kind": kind, "text": value})
}
