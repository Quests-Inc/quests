package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

// dropStagedFiles deletes the WebDAV copies of uploads among ids.
func (s *Server) dropStagedFiles(ctx context.Context, ops []store.DraftOp, ids []int64) {
	if !s.knowledgeHTTP().WebDAVOK() {
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
			_ = s.WebDAV.Delete(context.WithoutCancel(ctx), fp.Path)
		}
	}
}

// postGraphDraftFile stages an upload: scanned now, kept on WebDAV under
// drafts/, attached to its owner only when the draft is saved.
func (s *Server) postGraphDraftFile(w http.ResponseWriter, r *http.Request) {
	if !s.knowledgeHTTP().WebDAVOK() {
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	if !s.knowledgeHTTP().ClamAVOK() {
		writeErr(w, 503, "virus scanning is not configured (QUESTS_CLAMAV_ADDR)")
		return
	}
	maxBytes := s.MaxUploadBytes
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
	if !s.knowledgeHTTP().CheckQuota(w, r.Context(), int64(len(raw))) {
		return
	}
	declared := strings.TrimSpace(hdr.Header.Get("Content-Type"))
	detected := mimetype.Detect(raw).String()
	res, err := s.ClamAV.Scan(r.Context(), bytes.NewReader(raw))
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
	if err := s.WebDAV.Put(r.Context(), path, ctype, bytes.NewReader(raw), int64(len(raw))); err != nil {
		writeErr(w, 502, "file storage unavailable: "+err.Error())
		return
	}
	owner := strings.TrimSpace(r.FormValue("owner"))
	if owner != "" {
		if ref, ok := parseDraftRef(owner); !ok || (ref.Type != "quest" && ref.Type != "note" && ref.Type != "questline") {
			_ = s.WebDAV.Delete(context.WithoutCancel(r.Context()), path)
			writeErr(w, 422, "owner: bad reference")
			return
		}
	}
	payload, _ := json.Marshal(filePayload{
		Filename: filename, Size: int64(len(raw)), Mime: detected, Declared: declared, Path: path, Owner: owner,
	})
	op, err := s.Store.AddDraftOp(r.Context(), "upload_file", payload)
	if err != nil {
		_ = s.WebDAV.Delete(context.WithoutCancel(r.Context()), path)
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, op)
}

func (s *Server) postAttachmentOwner(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		OwnerType string `json:"owner_type"`
		OwnerID   int64  `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if in.OwnerType != ownerQuest && in.OwnerType != ownerQuestline && in.OwnerType != ownerNote {
		writeErr(w, 422, "owner_type must be quest, questline or note")
		return
	}
	ok, err := s.ownerExists(r.Context(), in.OwnerType, in.OwnerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "owner not found")
		return
	}
	before, err := s.Store.Knowledge().GetAttachment(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a, err := s.Store.SetAttachmentOwner(r.Context(), id, in.OwnerType, in.OwnerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.knowledgeHTTP().LogAttachment(r.Context(), "attachment_moved", a, fmt.Sprintf("перенесено: %s %d → %s %d", before.OwnerType, before.OwnerID, a.OwnerType, a.OwnerID))
	s.publishSilent(r.Context(), "attachment_updated", a.Filename, map[string]any{
		"attachment_id": a.ID, "owner_type": a.OwnerType, "owner_id": a.OwnerID,
	})
	writeJSON(w, 200, knowledge.AttachmentToRead(a))
}

// postStepMove moves a step to another quest (end of its steps). A quest keeps
// at least one step, like deleting does.
func (s *Server) postStepMove(w http.ResponseWriter, r *http.Request) {
	fromID, err1 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	stepID, err2 := strconv.ParseInt(r.PathValue("step_id"), 10, 64)
	if err1 != nil || err2 != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		QuestID int64 `json:"quest_id"`
		// AllowEmpty lets a quest end up with no steps for now; the draft
		// commit sets it after checking the final state of every quest.
		AllowEmpty bool `json:"allow_empty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.QuestID <= 0 {
		writeErr(w, 400, "quest_id required")
		return
	}
	if in.QuestID == fromID {
		writeErr(w, 422, "шаг уже в этом квесте")
		return
	}
	from, err := s.Store.Journal().GetQuest(r.Context(), fromID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	to, err := s.Store.Journal().GetQuest(r.Context(), in.QuestID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Target quest not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(from.Steps) <= 1 && !in.AllowEmpty {
		writeErr(w, 422, "нельзя унести последний шаг квеста")
		return
	}
	var moved journal.Step
	var rest []journal.Step
	for _, st := range from.Steps {
		if st.ID == stepID {
			moved = st
			continue
		}
		rest = append(rest, st)
	}
	if moved.ID == 0 {
		writeErr(w, 404, "Step not found")
		return
	}
	order := 0
	for _, st := range to.Steps {
		if st.SortOrder >= order {
			order = st.SortOrder + 1
		}
	}
	moved.QuestID, moved.SortOrder, moved.WaitPrevious = to.ID, order, false
	now := timeutil.NowUTC()
	from.Steps = rest
	to.Steps = append(to.Steps, moved)
	journal.SyncStatusFromSteps(&from, now)
	journal.SyncStatusFromSteps(&to, now)
	from.UpdatedAt, to.UpdatedAt = now, now
	if err := s.Store.Journal().MoveStep(r.Context(), stepID, from, to, order); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Step not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	for _, q := range []journal.Quest{from, to} {
		id := q.ID
		s.Hub.Publish("quest_updated", events.PublishOpts{
			Workspace: wsctx.From(r.Context()),
			QuestID:   &id, Title: q.Title, Detail: "перенос шага", Source: "web", Significance: string(q.Significance),
		})
	}
	nf, _ := s.Store.Journal().GetQuest(r.Context(), fromID)
	nt, _ := s.Store.Journal().GetQuest(r.Context(), in.QuestID)
	writeJSON(w, 200, map[string]any{
		"from": journal.ToQuestRead(nf, timeutil.NowUTC()),
		"to":   journal.ToQuestRead(nt, timeutil.NowUTC()),
	})
}

// ---- commit ----------------------------------------------------------------
