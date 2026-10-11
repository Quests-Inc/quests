package knowledgehttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// SafeFilename keeps the original name for humans but strips anything that
// could escape the storage directory. The stored name is uuid-prefixed anyway;
// this is belt and braces.
func SafeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	name = strings.TrimLeft(name, ".")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\x00', '\n', '\r':
			return -1
		}
		return r
	}, name)
	if name == "" {
		name = "file"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

func (h *Handler) PostAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
	if !h.WebDAVOK() {
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	// No scanner means no upload. Storing something unscanned and calling it
	// "pending" would leave a file nobody ever goes back to check.
	if !h.ClamAVOK() {
		writeErr(w, 503, "virus scanning is not configured (QUESTS_CLAMAV_ADDR)")
		return
	}
	ownerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ok, err := h.OwnerExists(r.Context(), ownerType, ownerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "owner not found")
		return
	}

	maxBytes := h.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 25 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, 400, "invalid multipart (or file too large)")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file required")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeErr(w, 400, "could not read upload")
		return
	}
	if len(raw) == 0 {
		writeErr(w, 400, "empty file")
		return
	}
	if int64(len(raw)) > maxBytes {
		writeErr(w, 413, fmt.Sprintf("file exceeds %d MB", maxBytes>>20))
		return
	}
	if !h.CheckQuota(w, r.Context(), int64(len(raw))) {
		return
	}

	declared := strings.TrimSpace(hdr.Header.Get("Content-Type"))
	detected := mimetype.Detect(raw).String()

	res, err := h.ClamAV.Scan(r.Context(), bytes.NewReader(raw))
	if err != nil {
		// Unknown is not clean: refuse instead of storing an unscanned file.
		writeErr(w, 503, "virus scan unavailable: "+err.Error())
		return
	}
	if res.Infected {
		writeErr(w, 422, "file rejected by virus scan: "+res.Signature)
		return
	}

	filename := SafeFilename(hdr.Filename)
	storedName := uuid.NewString() + "-" + filename
	webdavPath := fmt.Sprintf("attachments/%s-%d/%s", ownerType, ownerID, storedName)

	ctype := declared
	if ctype == "" {
		ctype = detected
	}
	if err := h.WebDAV.Put(r.Context(), webdavPath, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "upload to file server failed: "+err.Error())
		return
	}

	now := timeutil.NowUTC()
	a, err := h.Store.CreateAttachment(r.Context(), knowledge.Attachment{
		OwnerType:           ownerType,
		OwnerID:             ownerID,
		Filename:            filename,
		WebDAVPath:          webdavPath,
		SizeBytes:           int64(len(raw)),
		ContentTypeDeclared: declared,
		ContentTypeDetected: detected,
		Comment:             strings.TrimSpace(r.FormValue("comment")),
		UploadedAt:          now,
		ScanStatus:          "clean",
		ScannedAt:           &now,
	})
	if err != nil {
		// Don't leave bytes behind that no row points at.
		_ = h.WebDAV.Delete(context.WithoutCancel(r.Context()), webdavPath)
		writeErr(w, 500, err.Error())
		return
	}
	out := AttachmentView{AttachmentRead: knowledge.AttachmentToRead(a), AttachmentLive: &AttachmentLive{Available: true}}
	h.LogAttachment(r.Context(), "attachment_added", a, "загружено")
	h.publishSilent(r.Context(), "attachment_created", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	writeJSON(w, 201, out)
}
