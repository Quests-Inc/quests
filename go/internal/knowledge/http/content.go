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
	versioned := false
	if revStr := strings.TrimSpace(r.URL.Query().Get("revision")); revStr != "" {
		versioned = true
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
	thumbnail := 0
	if raw := r.URL.Query().Get("thumb"); raw != "" {
		thumbnail, _ = strconv.Atoi(raw)
		if thumbnail != 128 && thumbnail != 256 && thumbnail != 512 && thumbnail != 1024 && thumbnail != 2048 {
			writeErr(w, 422, "thumb must be 128, 256, 512, 1024 or 2048")
			return
		}
		etag = strings.TrimSuffix(etag, `"`) + fmt.Sprintf(`-thumb%d"`, thumbnail)
	}
	w.Header().Set("ETag", etag)
	if versioned {
		// The URL is pinned to a revision: its bytes never change, cache without revalidation.
		w.Header().Set("Cache-Control", "private, max-age=604800")
	} else {
		// The address of an image in a text never names a revision, so a cached copy could not tell that a newer one was
		// uploaded. The tag does: it is the card, the revision and its upload time, and a browser asks again each time.
		w.Header().Set("Cache-Control", "private, no-cache")
	}
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
	if thumbnail > 0 {
		h.serveThumbnail(w, r, pathToGet, etag, thumbnail)
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

// InlineAudioTypes is what <audio> may load. Anything else stays a download:
// the sniffed type is the allow-list, never the type the client declared.
var InlineAudioTypes = map[string]bool{
	"audio/mpeg":  true,
	"audio/mp4":   true,
	"audio/aac":   true,
	"audio/ogg":   true,
	"audio/wav":   true,
	"audio/wave":  true,
	"audio/x-wav": true,
	"audio/webm":  true,
	"audio/flac":  true,
	"audio/x-m4a": true,
}

// getAttachmentAudio is the sound counterpart of getAttachmentImage: one
// inline file, seekable. A Range header is forwarded to WebDAV; if the file
// server ignores it, the bytes are sliced here so the player can still seek.
func (h *Handler) getAttachmentAudio(w http.ResponseWriter, r *http.Request) {
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
	if a.Hidden && r.URL.Query().Get("preview") != "1" {
		writeErr(w, 404, "attachment not found")
		return
	}
	pathToGet := a.WebDAVPath
	detected := a.ContentTypeDetected
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
		detected = rev.ContentTypeDetected
		size = rev.SizeBytes
	}
	ctype, _, _ := strings.Cut(detected, ";")
	ctype = strings.ToLower(strings.TrimSpace(ctype))
	if !InlineAudioTypes[ctype] {
		writeErr(w, 415, "attachment is not a playable audio file")
		return
	}
	obj, err := h.WebDAV.Open(r.Context(), pathToGet, r.Header.Get("Range"))
	if errors.Is(err, webdav.ErrNotFound) {
		writeErr(w, 404, "file missing on the file server")
		return
	}
	if err != nil {
		writeErr(w, 502, "file server unreachable: "+err.Error())
		return
	}
	defer obj.Body.Close()

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Accept-Ranges", "bytes")
	if obj.Status == http.StatusRequestedRangeNotSatisfiable {
		if size > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		}
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if obj.Status == http.StatusPartialContent {
		if obj.Range != "" {
			w.Header().Set("Content-Range", obj.Range)
		}
		if obj.Length != "" {
			w.Header().Set("Content-Length", obj.Length)
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.Copy(w, obj.Body)
		return
	}
	rng := strings.TrimSpace(r.Header.Get("Range"))
	if rng != "" && size > 0 {
		start, end, ok := byteRange(rng, size)
		if !ok {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if _, err := io.CopyN(io.Discard, obj.Body, start); err != nil {
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.CopyN(w, obj.Body, end-start+1)
		return
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}

func byteRange(spec string, size int64) (int64, int64, bool) {
	if size <= 0 || !strings.HasPrefix(spec, "bytes=") || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	left, right, ok := strings.Cut(strings.TrimPrefix(spec, "bytes="), "-")
	if !ok {
		return 0, 0, false
	}
	if left == "" {
		n, err := strconv.ParseInt(right, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}
	start, err := strconv.ParseInt(left, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if right != "" {
		end, err = strconv.ParseInt(right, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
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
