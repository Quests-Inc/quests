package graphdraft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
	"github.com/quests-inc/quests/go/internal/store"
)

// dropStagedFiles deletes the WebDAV copies of uploads among ids.
func (h *Handler) dropStagedFiles(ctx context.Context, ops []store.DraftOp, ids []int64) {
	if !h.WebDAVOK() {
		return
	}
	del := map[int64]bool{}
	for _, id := range ids {
		del[id] = true
	}
	for _, o := range ops {
		if o.Kind != "upload_file" || !del[o.ID] {
			continue
		}
		var fp filePayload
		if json.Unmarshal(o.Payload, &fp) == nil && fp.Path != "" {
			_ = h.WebDAV.Delete(context.WithoutCancel(ctx), fp.Path)
		}
	}
}

// postGraphDraftFile stages an upload: scanned now, kept on WebDAV under
// drafts/, attached to its owner only when the draft is saved.
func (h *Handler) postGraphDraftFile(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || len(raw) == 0 {
		writeErr(w, 400, "empty or unreadable file")
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
	filename := knowledgehttp.SafeFilename(hdr.Filename)
	path := fmt.Sprintf("drafts/%s-%s", uuid.NewString(), filename)
	ctype := declared
	if ctype == "" {
		ctype = detected
	}
	if err := h.WebDAV.Put(r.Context(), path, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "file storage unavailable: "+err.Error())
		return
	}
	owner := strings.TrimSpace(r.FormValue("owner"))
	if owner != "" {
		if ref, ok := parseDraftRef(owner); !ok || (ref.Type != "quest" && ref.Type != "note" && ref.Type != "questline") {
			_ = h.WebDAV.Delete(context.WithoutCancel(r.Context()), path)
			writeErr(w, 422, "owner: bad reference")
			return
		}
	}
	payload, _ := json.Marshal(filePayload{
		Filename: filename, Size: int64(len(raw)), Mime: detected, Declared: declared, Path: path, Owner: owner,
	})
	op, err := h.Store.AddDraftOp(r.Context(), "upload_file", payload)
	if err != nil {
		_ = h.WebDAV.Delete(context.WithoutCancel(r.Context()), path)
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, op)
}

// ---- commit ----------------------------------------------------------------
