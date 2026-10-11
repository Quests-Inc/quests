package knowledgehttp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/webdav"
)

func (h *Handler) listAttachmentRevisions(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := h.attachmentFor(r, ownerType)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	revs, err := h.Store.ListAttachmentRevisions(r.Context(), a.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]knowledge.RevisionRead, 0, len(revs))
	for _, rev := range revs {
		out = append(out, knowledge.RevisionToRead(rev, a.CurrentRevision))
	}
	writeJSON(w, 200, AttachmentRevisions{AttachmentID: a.ID, CurrentRevision: a.CurrentRevision, Revisions: out})
}

func (h *Handler) postAttachmentRevision(w http.ResponseWriter, r *http.Request, ownerType string) {
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
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	if !h.ClamAVOK() {
		writeErr(w, 503, "virus scanning is not configured (QUESTS_CLAMAV_ADDR)")
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
		writeErr(w, 503, "virus scan unavailable: "+err.Error())
		return
	}
	if res.Infected {
		writeErr(w, 422, "file rejected by virus scan: "+res.Signature)
		return
	}

	filename := SafeFilename(hdr.Filename)
	if filename == "file" && a.Filename != "" {
		// Keep the card name when the client sends a generic/temp path.
		filename = a.Filename
	}
	storedName := uuid.NewString() + "-" + filename
	webdavPath := fmt.Sprintf("attachments/%s-%d/%s", a.OwnerType, a.OwnerID, storedName)
	ctype := declared
	if ctype == "" {
		ctype = detected
	}
	if err := h.WebDAV.Put(r.Context(), webdavPath, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "upload to file server failed: "+err.Error())
		return
	}

	now := timeutil.NowUTC()
	comment := strings.TrimSpace(r.FormValue("comment"))
	if comment == "" {
		comment = a.Comment
	}
	updated, dropPaths, err := h.Store.AddAttachmentRevision(r.Context(), a.ID, knowledge.AttachmentRevision{
		Filename:            filename,
		WebDAVPath:          webdavPath,
		SizeBytes:           int64(len(raw)),
		ContentTypeDeclared: declared,
		ContentTypeDetected: detected,
		Comment:             comment,
		UploadedAt:          now,
		ScanStatus:          "clean",
		ScannedAt:           &now,
	})
	if err != nil {
		_ = h.WebDAV.Delete(context.WithoutCancel(r.Context()), webdavPath)
		writeErr(w, 500, err.Error())
		return
	}
	for _, p := range dropPaths {
		if err := h.WebDAV.Delete(context.WithoutCancel(r.Context()), p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachment %d: rotated file left on webdav: %v\n", a.ID, err)
		}
	}
	out := AttachmentView{AttachmentRead: knowledge.AttachmentToRead(updated), AttachmentLive: &AttachmentLive{Available: true}}
	h.LogAttachment(r.Context(), "attachment_version", updated, fmt.Sprintf("новая версия v%d", updated.CurrentRevision))
	h.publishSilent(r.Context(), "attachment_updated", updated.Filename, map[string]any{
		"attachment_id": updated.ID,
		"owner_type":    updated.OwnerType,
		"owner_id":      updated.OwnerID,
		"revision":      updated.CurrentRevision,
	})
	writeJSON(w, 201, out)
}

func (h *Handler) deleteAttachmentRevision(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := h.attachmentFor(r, ownerType)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	revN, err := strconv.Atoi(r.PathValue("rev"))
	if err != nil || revN < 1 {
		writeErr(w, 422, "revision must be a positive integer")
		return
	}
	davPath, err := h.Store.DeleteAttachmentRevision(r.Context(), a.ID, revN)
	if errors.Is(err, knowledge.ErrConflict) {
		writeErr(w, 422, "cannot delete the current revision")
		return
	}
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "revision not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if h.WebDAVOK() {
		if err := h.WebDAV.Delete(r.Context(), davPath); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachment %d rev %d: file left on webdav: %v\n", a.ID, revN, err)
		}
	}
	h.LogAttachment(r.Context(), "attachment_version", a, fmt.Sprintf("удалена версия v%d", revN))
	h.publishSilent(r.Context(), "attachment_updated", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	w.WriteHeader(204)
}
