package knowledgehttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
)

func (h *Handler) RegisterAttachments(mux *http.ServeMux) {
	for _, owner := range []struct{ seg, typ string }{
		{"quests", ownerQuest},
		{"questlines", ownerQuestline},
		{"notes", ownerNote},
	} {
		seg, typ := owner.seg, owner.typ
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
			h.listAttachments(w, r, typ)
		})
		mux.HandleFunc("POST /api/"+seg+"/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
			h.PostAttachment(w, r, typ)
		})
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			h.getAttachmentContent(w, r, typ)
		})
		mux.HandleFunc("PATCH /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			h.patchAttachment(w, r, typ)
		})
		mux.HandleFunc("DELETE /api/"+seg+"/{id}/attachments/{aid}", func(w http.ResponseWriter, r *http.Request) {
			h.deleteAttachment(w, r, typ)
		})
		mux.HandleFunc("GET /api/"+seg+"/{id}/attachments/{aid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			h.listAttachmentRevisions(w, r, typ)
		})
		mux.HandleFunc("POST /api/"+seg+"/{id}/attachments/{aid}/revisions", func(w http.ResponseWriter, r *http.Request) {
			h.postAttachmentRevision(w, r, typ)
		})
		mux.HandleFunc("DELETE /api/"+seg+"/{id}/attachments/{aid}/revisions/{rev}", func(w http.ResponseWriter, r *http.Request) {
			h.deleteAttachmentRevision(w, r, typ)
		})
	}
	// Index of every attachment, grouped by owner. Default is metadata only
	// (no WebDAV Stat) so the journal can seed its cache in one cheap query.
	mux.HandleFunc("GET /api/attachments", h.listAllAttachments)
	// Owner-less inline view for markdown images: `![](attachment=N)`.
	mux.HandleFunc("GET /api/attachments/{aid}/image", h.getAttachmentImage)
}

// attachmentFor resolves {id}/{aid} and refuses an attachment that belongs to a
// different owner, so a valid id under the wrong quest is a 404, not a leak.
func (h *Handler) attachmentFor(r *http.Request, ownerType string) (knowledge.Attachment, error) {
	ownerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	aid, _ := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	a, err := h.Store.GetAttachment(r.Context(), aid)
	if err != nil {
		return knowledge.Attachment{}, err
	}
	if a.OwnerType != ownerType || a.OwnerID != ownerID {
		return knowledge.Attachment{}, knowledge.ErrNotFound
	}
	return a, nil
}

// LogAttachment records an attachment event against the attachment and its owner.
func (h *Handler) LogAttachment(ctx context.Context, kind string, a knowledge.Attachment, detail string) {
	h.LogEvent(context.WithoutCancel(ctx), ChangeEvent{
		Kind: kind, EntityType: "attachment", EntityID: a.ID,
		ParentType: a.OwnerType, ParentID: a.OwnerID, Title: a.Filename, Detail: detail,
	})
}

func (h *Handler) WebDAVOK() bool { return h.WebDAV != nil && h.WebDAV.Configured() }

func (h *Handler) ClamAVOK() bool { return h.ClamAV != nil && h.ClamAV.Configured() }

func (h *Handler) listAttachments(w http.ResponseWriter, r *http.Request, ownerType string) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rows, err := h.Store.ListAttachments(r.Context(), ownerType, id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	probe := r.URL.Query().Get("stat") != "0"
	out := make([]AttachmentView, 0, len(rows))
	for _, a := range rows {
		out = append(out, h.DecorateAttachment(r.Context(), a, probe))
	}
	writeJSON(w, 200, out)
}

// listAllAttachments is the journal cache seed: one database round-trip, grouped
// by owner. ?stat=1 opts into WebDAV probes (expensive with many files).
func (h *Handler) listAllAttachments(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListAllAttachments(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	probe := r.URL.Query().Get("stat") == "1"
	out := map[string]map[string][]AttachmentView{
		"quest":     {},
		"questline": {},
		"note":      {},
	}
	for _, a := range rows {
		bucket, ok := out[a.OwnerType]
		if !ok {
			continue
		}
		oid := strconv.FormatInt(a.OwnerID, 10)
		bucket[oid] = append(bucket[oid], h.DecorateAttachment(r.Context(), a, probe))
	}
	writeJSON(w, 200, out)
}

// sourceUpdatedSlack absorbs the gap between the upload timestamp (taken before
// the PUT) and the file server's own mtime, which may be second-granular.
const sourceUpdatedSlack = time.Minute

// DecorateAttachment adds live file-server facts. available is false (and the
// download link should hide) when WebDAV is down — not an error on the listing.
// source_updated is the "актуализируй" marker: the file's Last-Modified is
// newer than the moment the current revision was uploaded, i.e. the source
// file changed on the file server after we took it.
func (h *Handler) DecorateAttachment(ctx context.Context, a knowledge.Attachment, probe bool) AttachmentView {
	out := AttachmentView{AttachmentRead: knowledge.AttachmentToRead(a)}
	if !probe {
		// Leave available/source_updated off: unknown is not "missing", and a
		// bulk index must not paint every file as unavailable before Stat.
		return out
	}
	out.AttachmentLive = &AttachmentLive{}
	if !h.WebDAVOK() {
		return out
	}
	info, err := h.WebDAV.Stat(ctx, a.WebDAVPath)
	if err != nil {
		return out
	}
	out.Available = true
	if !info.LastModified.IsZero() {
		out.LastModified = timeutil.ToUTCISO(&info.LastModified)
		if info.LastModified.After(a.UploadedAt.UTC().Add(sourceUpdatedSlack)) {
			out.SourceUpdated = true
		}
	}
	return out
}

func (h *Handler) AttachmentsForOwner(ctx context.Context, ownerType string, ownerID int64, probe bool) []AttachmentView {
	rows, err := h.Store.ListAttachments(ctx, ownerType, ownerID)
	if err != nil {
		return []AttachmentView{}
	}
	out := make([]AttachmentView, 0, len(rows))
	for _, a := range rows {
		if a.Hidden {
			continue
		}
		out = append(out, h.DecorateAttachment(ctx, a, probe))
	}
	return out
}

// PurgeOwnerAttachments drops metadata first, then the bytes. A leftover file
// on WebDAV is inert; a row pointing at nothing is a broken link.
func (h *Handler) PurgeOwnerAttachments(ctx context.Context, ownerType string, ownerID int64) {
	paths, err := h.Store.AttachmentPathsForOwner(ctx, ownerType, ownerID)
	if err != nil {
		fmt.Printf("attachments: list paths for %s-%d: %v\n", ownerType, ownerID, err)
		return
	}
	if err := h.Store.DeleteAttachmentsForOwner(ctx, ownerType, ownerID); err != nil {
		fmt.Printf("attachments: delete rows for %s-%d: %v\n", ownerType, ownerID, err)
		return
	}
	if !h.WebDAVOK() {
		return
	}
	for _, p := range paths {
		if err := h.WebDAV.Delete(ctx, p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
			fmt.Printf("attachments: file left on webdav %s: %v\n", p, err)
		}
	}
}

func (h *Handler) patchAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
	a, err := h.attachmentFor(r, ownerType)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var body struct {
		Comment         *string `json:"comment"`
		CurrentRevision *int    `json:"current_revision"`
		Hidden          *bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	updated := a
	if body.CurrentRevision != nil {
		updated, err = h.Store.SetCurrentAttachmentRevision(r.Context(), a.ID, *body.CurrentRevision)
		if errors.Is(err, knowledge.ErrNotFound) {
			writeErr(w, 404, "revision not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.Comment != nil {
		updated, err = h.Store.SetAttachmentComment(r.Context(), a.ID, strings.TrimSpace(*body.Comment))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.Hidden != nil {
		updated, err = h.Store.SetAttachmentHidden(r.Context(), a.ID, *body.Hidden)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	if body.CurrentRevision != nil {
		h.LogAttachment(r.Context(), "attachment_version", updated, fmt.Sprintf("текущая версия v%d", updated.CurrentRevision))
	}
	if body.Hidden != nil && *body.Hidden != a.Hidden {
		if *body.Hidden {
			h.LogAttachment(r.Context(), "attachment_hidden", updated, "скрыто")
		} else {
			h.LogAttachment(r.Context(), "attachment_hidden", updated, "возвращено")
		}
	}
	if body.Comment == nil && body.CurrentRevision == nil && body.Hidden == nil {
		writeJSON(w, 200, knowledge.AttachmentToRead(a))
		return
	}
	h.publishSilent(r.Context(), "attachment_updated", updated.Filename, map[string]any{
		"attachment_id": updated.ID,
		"owner_type":    updated.OwnerType,
		"owner_id":      updated.OwnerID,
	})
	writeJSON(w, 200, knowledge.AttachmentToRead(updated))
}

func (h *Handler) deleteAttachment(w http.ResponseWriter, r *http.Request, ownerType string) {
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
	paths := make([]string, 0, len(revs)+1)
	seen := map[string]bool{}
	for _, rev := range revs {
		if !seen[rev.WebDAVPath] {
			seen[rev.WebDAVPath] = true
			paths = append(paths, rev.WebDAVPath)
		}
	}
	if a.WebDAVPath != "" && !seen[a.WebDAVPath] {
		paths = append(paths, a.WebDAVPath)
	}
	// Drop the row first: a leftover file on WebDAV is inert, a row pointing at
	// nothing is a broken link in the UI.
	if err := h.Store.DeleteAttachment(r.Context(), a.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if h.WebDAVOK() {
		for _, p := range paths {
			if err := h.WebDAV.Delete(r.Context(), p); err != nil && !errors.Is(err, webdav.ErrNotFound) {
				fmt.Printf("attachment %d: file left on webdav: %v\n", a.ID, err)
			}
		}
	}
	h.LogAttachment(r.Context(), "attachment_deleted", a, "удалено")
	h.publishSilent(r.Context(), "attachment_deleted", a.Filename, map[string]any{
		"attachment_id": a.ID,
		"owner_type":    a.OwnerType,
		"owner_id":      a.OwnerID,
	})
	w.WriteHeader(204)
}
