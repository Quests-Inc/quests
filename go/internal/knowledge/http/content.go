package knowledgehttp

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
)

func (h *Handler) getAttachmentContent(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := h.attachmentFor(r, ownerType)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !h.WebDAVOK() {
		writeErr(w, 503, "attachment storage is not configured")
		return
	}
	pathToGet := a.WebDAVPath
	filename := a.Filename
	size := a.SizeBytes
	if revStr := strings.TrimSpace(r.URL.Query().Get("revision")); revStr != "" {
		revN, err := strconv.Atoi(revStr)
		if err != nil || revN < 1 {
			writeErr(w, 422, "revision must be a positive integer")
			return
		}
		rev, err := h.Store.GetAttachmentRevision(r.Context(), a.ID, revN)
		if errors.Is(err, knowledge.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		pathToGet = rev.WebDAVPath
		filename = rev.Filename
		size = rev.SizeBytes
	}
	body, err := h.WebDAV.Get(r.Context(), pathToGet)
	if errors.Is(err, webdav.ErrNotFound) {
		writeErr(w, 404, "file missing on the file server")
		return
	}
	if err != nil {
		writeErr(w, 502, "file server unreachable: "+err.Error())
		return
	}
	defer body.Close()

	// Always a download, never something the browser renders or sniffs into
	// executing. This — not an extension blocklist — is what keeps a hostile
	// file harmless on the way out.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", ContentDisposition(filename))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}

// InlineImageTypes is the whole allow-list for the inline view. SVG is
// deliberately absent: it can carry script, and a raster-only rule is what
// lets the sanitizer trust `/api/attachments/N/image` as an <img> source.
var InlineImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// getAttachmentImage serves an attachment of any owner as an inline image.
// Unlike getAttachmentContent (always a download), this renders in <img>, so
// the type comes from the server-side sniff stored at upload (never from the
// client-declared one) and anything outside InlineImageTypes is refused.
func (h *Handler) getAttachmentImage(w http.ResponseWriter, r *http.Request) {
	aid, _ := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	a, err := h.Store.GetAttachment(r.Context(), aid)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !h.WebDAVOK() {
		writeErr(w, 503, "attachment storage is not configured")
		return
	}
	// Hidden cards live only on the Attachments tab: inline views 404 so a
	// markdown image falls back to the placeholder. ?preview=1 is the
	// Attachments tab's own thumbnail/preview and bypasses that.
	if a.Hidden && r.URL.Query().Get("preview") != "1" {
		writeErr(w, 404, "attachment not found")
		return
	}
	pathToGet := a.WebDAVPath
	detected := a.ContentTypeDetected
	size := a.SizeBytes
	revision, uploaded := a.CurrentRevision, a.UploadedAt
	if revStr := strings.TrimSpace(r.URL.Query().Get("revision")); revStr != "" {
		revN, err := strconv.Atoi(revStr)
		if err != nil || revN < 1 {
			writeErr(w, 422, "revision must be a positive integer")
			return
		}
		rev, err := h.Store.GetAttachmentRevision(r.Context(), a.ID, revN)
		if errors.Is(err, knowledge.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		pathToGet = rev.WebDAVPath
		detected = rev.ContentTypeDetected
		size = rev.SizeBytes
		revision, uploaded = rev.Revision, rev.UploadedAt
	}
	// The address of an image in a text never names a revision, so a cached copy could not tell that a newer one was
	// uploaded. The tag does: it is the card, the revision and its upload time, and a browser asks again each time.
	etag := fmt.Sprintf(`"att-%d-r%d-%d-%d"`, a.ID, revision, size, uploaded.Unix())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	// mimetype may append parameters ("image/png; charset=…"); compare the bare type.
	ctype, _, _ := strings.Cut(detected, ";")
	ctype = strings.ToLower(strings.TrimSpace(ctype))
	if !InlineImageTypes[ctype] {
		writeErr(w, 415, "attachment is not a raster image")
		return
	}
	body, err := h.WebDAV.Get(r.Context(), pathToGet)
	if errors.Is(err, webdav.ErrNotFound) {
		writeErr(w, 404, "file missing on the file server")
		return
	}
	if err != nil {
		writeErr(w, 502, "file server unreachable: "+err.Error())
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Even opened directly in a tab the response can't run anything.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", "inline")
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}

// ContentDisposition sends an ASCII fallback plus RFC 5987 UTF-8, so a Russian
// filename survives without breaking older clients.
func ContentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		ascii, urlEscapePath(name))
}

func urlEscapePath(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
