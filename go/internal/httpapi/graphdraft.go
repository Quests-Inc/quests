package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/refs"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
)

// The relations map is edited through a draft: an ordered log of operations
// kept on the server. Nothing in it touches real data until /commit, and any
// single operation can be dropped without losing the rest.

var draftKinds = map[string]bool{
	"create_quest": true,
	"create_note":  true,
	"create_step":  true,
	"upload_file":  true,
	"add_ref":      true,
	"remove_ref":   true,
	"attach":       true,
	"edit_quest":   true,
	"edit_step":    true,
	"edit_note":    true,
	"copy":         true,
}

// draftRef is "type:id" where id is a number (real entity) or "t<op>" (made
// by an op of this draft).
var draftRefRe = regexp.MustCompile(`^(quest|note|step|attachment|questline):(\d+|t\d+)$`)

type draftRef struct {
	Type string
	ID   string
}

func parseDraftRef(s string) (draftRef, bool) {
	m := draftRefRe.FindStringSubmatch(s)
	if m == nil {
		return draftRef{}, false
	}
	return draftRef{Type: m[1], ID: m[2]}, true
}

func (r draftRef) isTmp() bool { return strings.HasPrefix(r.ID, "t") }
func (r draftRef) String() string {
	return r.Type + ":" + r.ID
}

// payload shapes
type createPayload struct {
	Fields map[string]any `json:"fields"`
	// Quest owns a step (create_step); Owner owns an uploaded file.
	Quest string `json:"quest,omitempty"`
}

type filePayload struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Mime     string `json:"mime"`
	Declared string `json:"declared,omitempty"`
	Path     string `json:"webdav_path"`
	Owner    string `json:"owner,omitempty"`
}

type editPayload struct {
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields"`
}

type copyPayload struct {
	Source string `json:"source"`
}

type linkPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type attachPayload struct {
	Item  string `json:"item"`
	Owner string `json:"owner"`
}

func (s *Server) registerGraphDraft(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/graph-draft", s.getGraphDraft)
	mux.HandleFunc("POST /api/graph-draft/ops", s.postGraphDraftOp)
	mux.HandleFunc("PUT /api/graph-draft/ops/{id}", s.putGraphDraftOp)
	mux.HandleFunc("DELETE /api/graph-draft/ops/{id}", s.deleteGraphDraftOp)
	mux.HandleFunc("PUT /api/graph-draft/roots", s.putGraphDraftRoots)
	mux.HandleFunc("POST /api/graph-draft/files", s.postGraphDraftFile)
	mux.HandleFunc("DELETE /api/graph-draft", s.cancelGraphDraft)
	mux.HandleFunc("POST /api/graph-draft/commit", s.commitGraphDraft)
	mux.HandleFunc("POST /api/attachments/{id}/owner", s.postAttachmentOwner)
	mux.HandleFunc("POST /api/quests/{id}/steps/{step_id}/move", s.postStepMove)
}

func (s *Server) getGraphDraft(w http.ResponseWriter, r *http.Request) {
	ops, err := s.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	roots, _ := s.Store.DraftRoots(r.Context())
	writeJSON(w, 200, map[string]any{"ops": ops, "roots": roots})
}

// refExists tells whether a draft reference points at something real now or
// at an op already in the draft.
func (s *Server) draftRefExists(ctx context.Context, ref draftRef, ops []store.DraftOp) bool {
	if ref.isTmp() {
		id, _ := strconv.ParseInt(ref.ID[1:], 10, 64)
		for _, o := range ops {
			if o.ID != id {
				continue
			}
			switch o.Kind {
			case "create_quest":
				return ref.Type == "quest"
			case "create_note":
				return ref.Type == "note"
			case "create_step":
				return ref.Type == "step"
			case "upload_file":
				return ref.Type == "attachment"
			case "copy":
				var cp copyPayload
				if json.Unmarshal(o.Payload, &cp) == nil {
					if src, ok := parseDraftRef(cp.Source); ok {
						return ref.Type == src.Type
					}
				}
			}
		}
		return false
	}
	id, _ := strconv.ParseInt(ref.ID, 10, 64)
	switch ref.Type {
	case "quest":
		_, err := s.Store.GetQuest(ctx, id)
		return err == nil
	case "note":
		ok, err := s.Store.NoteExists(ctx, id)
		return err == nil && ok
	case "questline":
		_, err := s.Store.GetQuestline(ctx, id)
		return err == nil
	case "step":
		_, err := s.Store.QuestIDForStep(ctx, id)
		return err == nil
	case "attachment":
		_, err := s.Store.GetAttachment(ctx, id)
		return err == nil
	}
	return false
}

func (s *Server) postGraphDraftOp(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if !draftKinds[in.Kind] || in.Kind == "upload_file" {
		writeErr(w, 422, "unknown op kind")
		return
	}
	ops, err := s.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if msg := s.validateDraftOp(r.Context(), in.Kind, in.Payload, ops); msg != "" {
		writeErr(w, 422, msg)
		return
	}
	op, err := s.Store.AddDraftOp(r.Context(), in.Kind, in.Payload)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, op)
}

// validateDraftOp checks shape and that referenced things exist. Whether the
// draft as a whole can be saved is decided at commit.
func (s *Server) validateDraftOp(ctx context.Context, kind string, raw json.RawMessage, ops []store.DraftOp) string {
	needRef := func(v, what string, types ...string) string {
		ref, ok := parseDraftRef(v)
		if !ok {
			return what + ": bad reference"
		}
		okType := len(types) == 0
		for _, t := range types {
			okType = okType || t == ref.Type
		}
		if !okType {
			return what + ": unsupported type " + ref.Type
		}
		if !s.draftRefExists(ctx, ref, ops) {
			return what + ": not found"
		}
		return ""
	}
	switch kind {
	case "create_quest", "create_note", "create_step":
		var p createPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "invalid payload"
		}
		if title, _ := p.Fields["title"].(string); strings.TrimSpace(title) == "" {
			return "title required"
		}
		if kind == "create_step" && p.Quest != "" {
			return needRef(p.Quest, "quest", "quest")
		}
	case "edit_quest", "edit_step", "edit_note":
		var p editPayload
		if err := json.Unmarshal(raw, &p); err != nil || len(p.Fields) == 0 {
			return "invalid payload"
		}
		want := map[string]string{"edit_quest": "quest", "edit_step": "step", "edit_note": "note"}[kind]
		ref, ok := parseDraftRef(p.ID)
		if !ok || ref.Type != want || ref.isTmp() {
			return "id: only saved " + want + "s can be edited this way"
		}
		return needRef(p.ID, "id", want)
	case "copy":
		var p copyPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "invalid payload"
		}
		ref, ok := parseDraftRef(p.Source)
		if !ok || ref.isTmp() {
			return "source: only saved things can be copied"
		}
		return needRef(p.Source, "source")
	case "add_ref", "remove_ref":
		var p linkPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "invalid payload"
		}
		if m := needRef(p.From, "from", "quest", "note", "step", "questline"); m != "" {
			return m
		}
		if m := needRef(p.To, "to"); m != "" {
			return m
		}
		if p.From == p.To {
			return "a thing cannot link to itself"
		}
		has := s.draftHasRef(ctx, p.From, p.To, ops)
		if kind == "add_ref" && has {
			return "ссылка уже есть"
		}
		if kind == "remove_ref" && !has {
			return "такой ссылки нет"
		}
	case "attach":
		var p attachPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "invalid payload"
		}
		item, ok := parseDraftRef(p.Item)
		if !ok || (item.Type != "attachment" && item.Type != "step") {
			return "item: only attachments and new steps can be attached"
		}
		if m := needRef(p.Item, "item"); m != "" {
			return m
		}
		if item.Type == "step" {
			return needRef(p.Owner, "owner", "quest")
		}
		return needRef(p.Owner, "owner", "quest", "note", "questline")
	}
	return ""
}

// draftHasRef tells whether `from` links to `to` once the draft so far is
// applied: its saved text first, then each op in order.
func (s *Server) draftHasRef(ctx context.Context, from, to string, ops []store.DraftOp) bool {
	f, ok1 := parseDraftRef(from)
	t, ok2 := parseDraftRef(to)
	if !ok1 || !ok2 {
		return false
	}
	has := false
	textHas := func(text string) bool {
		if t.isTmp() {
			return false
		}
		tid, _ := strconv.ParseInt(t.ID, 10, 64)
		for _, r := range refs.Parse(text) {
			if r.Kind == t.Type && r.ID == tid {
				return true
			}
		}
		return false
	}
	if !f.isTmp() {
		id, _ := strconv.ParseInt(f.ID, 10, 64)
		switch f.Type {
		case "quest":
			if q, err := s.Store.GetQuest(ctx, id); err == nil {
				has = textHas(q.Description)
			}
		case "note":
			if n, err := s.Store.GetNote(ctx, id); err == nil {
				has = textHas(n.Description)
			}
		case "step":
			if qid, err := s.Store.QuestIDForStep(ctx, id); err == nil {
				if q, err := s.Store.GetQuest(ctx, qid); err == nil {
					for _, st := range q.Steps {
						if st.ID == id {
							has = textHas(st.Description)
						}
					}
				}
			}
		case "questline":
			if ql, err := s.Store.GetQuestline(ctx, id); err == nil {
				d, _ := ql["description"].(string)
				has = textHas(d)
			}
		}
	}
	for _, o := range ops {
		switch o.Kind {
		case "create_quest", "create_note", "create_step":
			if f.isTmp() && f.ID == store.TmpID(o.ID) {
				var p createPayload
				_ = json.Unmarshal(o.Payload, &p)
				desc, _ := p.Fields["description"].(string)
				has = has || textHas(desc)
			}
		case "add_ref", "remove_ref":
			var p linkPayload
			if json.Unmarshal(o.Payload, &p) == nil && p.From == from && p.To == to {
				has = o.Kind == "add_ref"
			}
		}
	}
	return has
}

// opRefs lists the draft references an op points at (the fields that name
// other things), parsed rather than searched for, so free text can never be
// mistaken for a reference.
func opRefs(o store.DraftOp) []string {
	var p struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Item   string `json:"item"`
		Owner  string `json:"owner"`
		Quest  string `json:"quest"`
		ID     string `json:"id"`
		Source string `json:"source"`
	}
	if json.Unmarshal(o.Payload, &p) != nil {
		return nil
	}
	var out []string
	for _, v := range []string{p.From, p.To, p.Item, p.Owner, p.Quest, p.ID, p.Source} {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// createdRef is the placeholder an op's created entity answers to, or "".
func createdRef(o store.DraftOp) string {
	switch o.Kind {
	case "create_quest":
		return "quest:" + store.TmpID(o.ID)
	case "create_note":
		return "note:" + store.TmpID(o.ID)
	case "create_step":
		return "step:" + store.TmpID(o.ID)
	case "upload_file":
		return "attachment:" + store.TmpID(o.ID)
	case "copy":
		var p copyPayload
		if json.Unmarshal(o.Payload, &p) == nil {
			if src, ok := parseDraftRef(p.Source); ok {
				return src.Type + ":" + store.TmpID(o.ID)
			}
		}
	}
	return ""
}

// dependents returns the ids of ops that cannot stand once op id is gone, in
// addition to id itself: ops that use what it created (transitively), and the
// op right after it that only undoes or repeats the same link.
func dependents(ops []store.DraftOp, id int64) []int64 {
	byID := map[int64]store.DraftOp{}
	for _, o := range ops {
		byID[o.ID] = o
	}
	out := []int64{id}
	gone := map[int64]bool{id: true}
	// Ops dropped only because they paired with a dropped one do not pair
	// onward: what follows them still stands on its own.
	paired := map[int64]bool{}
	drop := func(o store.DraftOp) {
		gone[o.ID] = true
		out = append(out, o.ID)
	}
	linkOf := func(o store.DraftOp) (linkPayload, bool) {
		var p linkPayload
		if (o.Kind != "add_ref" && o.Kind != "remove_ref") || json.Unmarshal(o.Payload, &p) != nil {
			return p, false
		}
		return p, true
	}
	for changed := true; changed; {
		changed = false
		for _, o := range ops {
			if gone[o.ID] {
				continue
			}
			uses := false
			for _, r := range opRefs(o) {
				for g := range gone {
					if c := createdRef(byID[g]); c != "" && c == r {
						uses = true
					}
				}
			}
			if uses {
				drop(o)
				changed = true
			}
		}
		// Without an add, the next remove of that link has nothing to remove
		// (and without a remove, the next add has nothing to add back). Only
		// the next one: what follows still makes sense on its own.
		for g := range gone {
			if paired[g] {
				continue
			}
			gp, ok := linkOf(byID[g])
			if !ok {
				continue
			}
			for _, o := range ops {
				if o.ID <= g || gone[o.ID] {
					continue
				}
				p, ok := linkOf(o)
				if !ok || p.From != gp.From || p.To != gp.To {
					continue
				}
				if o.Kind != byID[g].Kind {
					drop(o)
					paired[o.ID] = true
					changed = true
				}
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Server) deleteGraphDraftOp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	ops, err := s.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	found := false
	for _, o := range ops {
		found = found || o.ID == id
	}
	if !found {
		writeErr(w, 404, "op not found")
		return
	}
	ids := dependents(ops, id)
	if r.URL.Query().Get("dry") != "" {
		writeJSON(w, 200, map[string]any{"would_remove": ids})
		return
	}
	s.dropStagedFiles(r.Context(), ops, ids)
	if err := s.Store.DeleteDraftOps(r.Context(), ids); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"removed": ids})
}

// dropStagedFiles deletes the WebDAV copies of uploads among ids.
func (s *Server) dropStagedFiles(ctx context.Context, ops []store.DraftOp, ids []int64) {
	if !s.webdavOK() {
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

func (s *Server) putGraphDraftRoots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Roots []struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		} `json:"roots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	for _, x := range in.Roots {
		if x.Type != "quest" && x.Type != "note" && x.Type != "questline" || x.ID <= 0 {
			writeErr(w, 422, "bad root")
			return
		}
	}
	raw, _ := json.Marshal(in.Roots)
	if in.Roots == nil {
		raw = []byte("[]")
	}
	if err := s.Store.SetDraftRoots(r.Context(), raw); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"roots": json.RawMessage(raw)})
}

func (s *Server) cancelGraphDraft(w http.ResponseWriter, r *http.Request) {
	ops, err := s.Store.ClearDraft(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids := make([]int64, len(ops))
	for i, o := range ops {
		ids[i] = o.ID
	}
	s.dropStagedFiles(r.Context(), ops, ids)
	w.WriteHeader(204)
}

// postGraphDraftFile stages an upload: scanned now, kept on WebDAV under
// drafts/, attached to its owner only when the draft is saved.
func (s *Server) postGraphDraftFile(w http.ResponseWriter, r *http.Request) {
	if !s.webdavOK() {
		writeErr(w, 503, "attachment storage is not configured (QUESTS_WEBDAV_URL)")
		return
	}
	if !s.clamavOK() {
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
	filename := safeFilename(hdr.Filename)
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
	before, err := s.Store.GetAttachment(r.Context(), id)
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
	s.logAttachment("attachment_moved", a, fmt.Sprintf("перенесено: %s %d → %s %d", before.OwnerType, before.OwnerID, a.OwnerType, a.OwnerID))
	s.publishSilent("attachment_updated", a.Filename, map[string]any{
		"attachment_id": a.ID, "owner_type": a.OwnerType, "owner_id": a.OwnerID,
	})
	writeJSON(w, 200, store.AttachmentToRead(a))
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
	from, err := s.Store.GetQuest(r.Context(), fromID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Quest not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	to, err := s.Store.GetQuest(r.Context(), in.QuestID)
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
	var moved domain.Step
	var rest []domain.Step
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
	domain.SyncStatusFromSteps(&from, now)
	domain.SyncStatusFromSteps(&to, now)
	from.UpdatedAt, to.UpdatedAt = now, now
	if err := s.Store.MoveStep(r.Context(), stepID, from, to, order); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Step not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	for _, q := range []domain.Quest{from, to} {
		id := q.ID
		s.Hub.Publish("quest_updated", events.PublishOpts{
			QuestID: &id, Title: q.Title, Detail: "перенос шага", Source: "web", Significance: string(q.Significance),
		})
	}
	nf, _ := s.Store.GetQuest(r.Context(), fromID)
	nt, _ := s.Store.GetQuest(r.Context(), in.QuestID)
	writeJSON(w, 200, map[string]any{
		"from": domain.ToQuestRead(nf, timeutil.NowUTC()),
		"to":   domain.ToQuestRead(nt, timeutil.NowUTC()),
	})
}

// ---- commit ----------------------------------------------------------------

type draftProblem struct {
	Op      int64  `json:"op"`
	Message string `json:"message"`
}

func (s *Server) commitGraphDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ops, err := s.Store.ListDraftOps(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(ops) == 0 {
		writeJSON(w, 200, map[string]any{"created": map[string]int64{}, "applied": 0})
		return
	}
	owners, problems := s.planDraft(ctx, ops)
	if len(problems) > 0 {
		writeJSON(w, 422, map[string]any{"error": "draft cannot be saved", "problems": problems})
		return
	}
	mux := s.routes()
	created := map[string]int64{}
	var applied []int64
	fail := func(op store.DraftOp, err error) {
		// Keep what failed and what follows; swap placeholders that already
		// became real entities so a retry does not create them twice.
		_ = s.Store.DeleteDraftOps(ctx, applied)
		rest, _ := s.Store.ListDraftOps(ctx)
		for _, o := range rest {
			text := string(o.Payload)
			for k, id := range created {
				text = strings.ReplaceAll(text, `"`+k+`"`, `"`+strings.Split(k, ":")[0]+":"+strconv.FormatInt(id, 10)+`"`)
			}
			if text != string(o.Payload) {
				_ = s.Store.UpdateDraftOpPayload(ctx, o.ID, json.RawMessage(text))
			}
		}
		writeJSON(w, 500, map[string]any{
			"error": err.Error(), "failed_op": op.ID, "applied": applied, "created": created,
		})
	}

	// real turns "quest:t5" / "quest:7" into the final type and id.
	real := func(v string) (string, int64, bool) {
		ref, ok := parseDraftRef(v)
		if !ok {
			return "", 0, false
		}
		if ref.isTmp() {
			id, ok := created[ref.String()]
			return ref.Type, id, ok
		}
		id, _ := strconv.ParseInt(ref.ID, 10, 64)
		return ref.Type, id, true
	}
	collection := map[string]string{"quest": "quests", "note": "notes", "questline": "questlines"}

	// Pass 1: things that come into being, containers before their contents.
	for pass := 1; pass <= 3; pass++ {
		for _, op := range ops {
			if passOf(op) != pass {
				continue
			}
			key := createdRef(op)
			switch op.Kind {
			case "create_quest", "create_note":
				var p createPayload
				_ = json.Unmarshal(op.Payload, &p)
				path := "/api/quests"
				if op.Kind == "create_note" {
					path = "/api/notes"
				}
				body, _ := json.Marshal(p.Fields)
				code, out := dispatch(ctx, mux, "POST", path, "application/json", body)
				id, derr := idFrom(code, out)
				if derr != nil {
					fail(op, derr)
					return
				}
				created[key] = id
			case "create_step":
				var p createPayload
				_ = json.Unmarshal(op.Payload, &p)
				_, qid, ok := real(owners[key])
				if !ok {
					fail(op, errors.New("step has no quest"))
					return
				}
				body, _ := json.Marshal(p.Fields)
				code, out := dispatch(ctx, mux, "POST", fmt.Sprintf("/api/quests/%d/steps", qid), "application/json", body)
				if code >= 300 {
					fail(op, errors.New(errText(code, out)))
					return
				}
				// The response is the quest; the new step is the last by order.
				sid, derr := lastStepID(out)
				if derr != nil {
					fail(op, derr)
					return
				}
				created[key] = sid
			case "upload_file":
				var fp filePayload
				_ = json.Unmarshal(op.Payload, &fp)
				otype, oid, ok := real(owners[key])
				if !ok {
					fail(op, errors.New("file has no owner"))
					return
				}
				rc, gerr := s.WebDAV.Get(ctx, fp.Path)
				if gerr != nil {
					fail(op, fmt.Errorf("staged file missing: %w", gerr))
					return
				}
				data, _ := io.ReadAll(rc)
				_ = rc.Close()
				ctype := fp.Declared
				if ctype == "" {
					ctype = fp.Mime
				}
				id, uerr := postAttachment(ctx, mux, collection[otype], oid, fp.Filename, ctype, "", data)
				if uerr != nil {
					fail(op, uerr)
					return
				}
				created[key] = id
				_ = s.WebDAV.Delete(context.WithoutCancel(ctx), fp.Path)
			case "copy":
				id, cerr := s.copyEntity(ctx, mux, op, owners[key], real, collection)
				if cerr != nil {
					fail(op, cerr)
					return
				}
				created[key] = id
			}
			applied = append(applied, op.ID)
		}
	}

	// Pass 2: changes to things that already exist (or just came into being).
	for _, op := range ops {
		switch op.Kind {
		case "add_ref", "remove_ref":
			var p linkPayload
			_ = json.Unmarshal(op.Payload, &p)
			ft, fid, ok1 := real(p.From)
			tt, tid, ok2 := real(p.To)
			if !ok1 || !ok2 {
				fail(op, errors.New("link endpoint missing"))
				return
			}
			if err := s.editRef(ctx, mux, op.Kind == "add_ref", ft, fid, fmt.Sprintf("%s=%d", tt, tid)); err != nil {
				fail(op, err)
				return
			}
		case "edit_quest", "edit_step", "edit_note":
			var p editPayload
			_ = json.Unmarshal(op.Payload, &p)
			_, id, ok := real(p.ID)
			if !ok {
				fail(op, errors.New("edit target missing"))
				return
			}
			path := fmt.Sprintf("/api/quests/%d", id)
			if op.Kind == "edit_note" {
				path = fmt.Sprintf("/api/notes/%d", id)
			}
			if op.Kind == "edit_step" {
				qid, qerr := s.Store.QuestIDForStep(ctx, id)
				if qerr != nil {
					fail(op, qerr)
					return
				}
				path = fmt.Sprintf("/api/quests/%d/steps/%d", qid, id)
			}
			body, _ := json.Marshal(p.Fields)
			if code, out := dispatch(ctx, mux, "PATCH", path, "application/json", body); code >= 300 {
				fail(op, errors.New(errText(code, out)))
				return
			}
		case "attach":
			var p attachPayload
			_ = json.Unmarshal(op.Payload, &p)
			item, _ := parseDraftRef(p.Item)
			if item.isTmp() { // handled when the thing was created
				break
			}
			ot, oid, ok := real(p.Owner)
			if !ok {
				fail(op, errors.New("owner missing"))
				return
			}
			aid, _ := strconv.ParseInt(item.ID, 10, 64)
			if item.Type == "step" {
				fromQ, qerr := s.Store.QuestIDForStep(ctx, aid)
				if qerr != nil {
					fail(op, qerr)
					return
				}
				body, _ := json.Marshal(map[string]any{"quest_id": oid, "allow_empty": true})
				code, out := dispatch(ctx, mux, "POST", fmt.Sprintf("/api/quests/%d/steps/%d/move", fromQ, aid), "application/json", body)
				if code >= 300 {
					fail(op, errors.New(errText(code, out)))
					return
				}
				break
			}
			body, _ := json.Marshal(map[string]any{"owner_type": ot, "owner_id": oid})
			code, out := dispatch(ctx, mux, "POST", fmt.Sprintf("/api/attachments/%d/owner", aid), "application/json", body)
			if code >= 300 {
				fail(op, errors.New(errText(code, out)))
				return
			}
		default:
			continue
		}
		applied = append(applied, op.ID)
	}

	all := make([]int64, len(ops))
	for i, o := range ops {
		all[i] = o.ID
	}
	if err := s.Store.DeleteDraftOps(ctx, all); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"created": created, "applied": len(ops)})
}

// planDraft works out who owns each new step and file and reports what stops
// the draft from being saved.
func (s *Server) planDraft(ctx context.Context, ops []store.DraftOp) (map[string]string, []draftProblem) {
	owners := map[string]string{} // "step:t3" → "quest:5"
	var problems []draftProblem
	for _, op := range ops {
		switch op.Kind {
		case "create_step":
			var p createPayload
			_ = json.Unmarshal(op.Payload, &p)
			if p.Quest != "" {
				owners["step:"+store.TmpID(op.ID)] = p.Quest
			}
		case "upload_file":
			var p filePayload
			_ = json.Unmarshal(op.Payload, &p)
			if p.Owner != "" {
				owners["attachment:"+store.TmpID(op.ID)] = p.Owner
			}
		}
	}
	// A copy of a step or file stays with the original's owner until attached elsewhere.
	for _, op := range ops {
		if op.Kind != "copy" {
			continue
		}
		var p copyPayload
		_ = json.Unmarshal(op.Payload, &p)
		src, ok := parseDraftRef(p.Source)
		if !ok || src.isTmp() {
			continue
		}
		sid, _ := strconv.ParseInt(src.ID, 10, 64)
		switch src.Type {
		case "step":
			if qid, err := s.Store.QuestIDForStep(ctx, sid); err == nil {
				owners["step:"+store.TmpID(op.ID)] = fmt.Sprintf("quest:%d", qid)
			}
		case "attachment":
			if a, err := s.Store.GetAttachment(ctx, sid); err == nil {
				owners["attachment:"+store.TmpID(op.ID)] = fmt.Sprintf("%s:%d", a.OwnerType, a.OwnerID)
			}
		}
	}
	for _, op := range ops { // later attaches win
		if op.Kind != "attach" {
			continue
		}
		var p attachPayload
		_ = json.Unmarshal(op.Payload, &p)
		if item, ok := parseDraftRef(p.Item); ok && item.isTmp() {
			owners[item.String()] = p.Owner
		}
	}
	// Final state of every real quest touched by step moves: each keeps a step.
	// Taking a quest's last step is allowed in the draft as long as another
	// step comes in before saving.
	finalOwner := map[int64]int64{} // real step → real quest it ends up in
	lastMoveOp := map[int64]int64{} // origin quest → an op that takes a step from it
	for _, op := range ops {
		if op.Kind != "attach" {
			continue
		}
		var p attachPayload
		_ = json.Unmarshal(op.Payload, &p)
		item, ok := parseDraftRef(p.Item)
		owner, ok2 := parseDraftRef(p.Owner)
		if !ok || !ok2 || item.Type != "step" || item.isTmp() || owner.isTmp() {
			continue
		}
		sid, _ := strconv.ParseInt(item.ID, 10, 64)
		oid, _ := strconv.ParseInt(owner.ID, 10, 64)
		finalOwner[sid] = oid
		if origin, err := s.Store.QuestIDForStep(ctx, sid); err == nil {
			lastMoveOp[origin] = op.ID
		}
	}
	delta := map[int64]int{}
	for sid, dest := range finalOwner {
		origin, err := s.Store.QuestIDForStep(ctx, sid)
		if err != nil || origin == dest {
			continue
		}
		delta[origin]--
		delta[dest]++
	}
	for key, owner := range owners { // new steps landing in real quests
		if strings.HasPrefix(key, "step:t") {
			if ref, ok := parseDraftRef(owner); ok && !ref.isTmp() {
				id, _ := strconv.ParseInt(ref.ID, 10, 64)
				delta[id]++
			}
		}
	}
	for qid, d := range delta {
		if d >= 0 {
			continue
		}
		q, err := s.Store.GetQuest(ctx, qid)
		if err == nil && len(q.Steps)+d < 1 {
			problems = append(problems, draftProblem{lastMoveOp[qid], "квест «" + q.Title + "» останется без шагов"})
		}
	}
	for _, op := range ops {
		switch op.Kind {
		case "create_step":
			if owners["step:"+store.TmpID(op.ID)] == "" {
				problems = append(problems, draftProblem{op.ID, "шаг не прикреплён к квесту"})
			}
		case "upload_file":
			if owners["attachment:"+store.TmpID(op.ID)] == "" {
				problems = append(problems, draftProblem{op.ID, "файл не прикреплён к владельцу"})
			}
		}
	}
	return owners, problems
}

// editRef appends or removes a "type=id" token in an entity's description.
func (s *Server) editRef(ctx context.Context, mux http.Handler, add bool, typ string, id int64, token string) error {
	var desc, path string
	switch typ {
	case "quest":
		q, err := s.Store.GetQuest(ctx, id)
		if err != nil {
			return err
		}
		desc, path = q.Description, fmt.Sprintf("/api/quests/%d", id)
	case "note":
		n, err := s.Store.GetNote(ctx, id)
		if err != nil {
			return err
		}
		desc, path = n.Description, fmt.Sprintf("/api/notes/%d", id)
	case "questline":
		ql, err := s.Store.GetQuestline(ctx, id)
		if err != nil {
			return err
		}
		desc, _ = ql["description"].(string)
		path = fmt.Sprintf("/api/questlines/%d", id)
	case "step":
		qid, err := s.Store.QuestIDForStep(ctx, id)
		if err != nil {
			return err
		}
		q, err := s.Store.GetQuest(ctx, qid)
		if err != nil {
			return err
		}
		for _, st := range q.Steps {
			if st.ID == id {
				desc = st.Description
			}
		}
		path = fmt.Sprintf("/api/quests/%d/steps/%d", qid, id)
	default:
		return fmt.Errorf("cannot edit text of %s", typ)
	}
	next := editToken(desc, token, add)
	if next == desc {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"description": next})
	code, out := dispatch(ctx, mux, "PATCH", path, "application/json", body)
	if code >= 300 {
		return errors.New(errText(code, out))
	}
	return nil
}

// editToken adds token as its own paragraph, or strips every occurrence of it.
func editToken(desc, token string, add bool) string {
	kind, num, _ := strings.Cut(token, "=")
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(kind) + `=` + regexp.QuoteMeta(num) + `\b`)
	has := false
	for _, r := range refs.Parse(desc) {
		if r.Kind == kind && strconv.FormatInt(r.ID, 10) == num {
			has = true
		}
	}
	if add {
		if has {
			return desc
		}
		if strings.TrimSpace(desc) == "" {
			return token
		}
		return strings.TrimRight(desc, " \n") + "\n\n" + token
	}
	out := re.ReplaceAllString(desc, "")
	out = regexp.MustCompile(`[ \t]+\n`).ReplaceAllString(out, "\n")
	out = regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
	return strings.TrimSpace(out)
}

// dispatch runs a request through the API routes without the auth layer: the
// commit is already an authenticated action, and going through the handlers
// keeps validation, change log and live events identical to a manual edit.
func dispatch(ctx context.Context, h http.Handler, method, path, ctype string, body []byte) (int, []byte) {
	req := httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func idFrom(code int, body []byte) (int64, error) {
	if code >= 300 {
		return 0, errors.New(errText(code, body))
	}
	var v struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.ID == 0 {
		return 0, errors.New("no id in response")
	}
	return v.ID, nil
}

func lastStepID(body []byte) (int64, error) {
	var q struct {
		Steps []struct {
			ID int64 `json:"id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(body, &q); err != nil || len(q.Steps) == 0 {
		return 0, errors.New("no steps in response")
	}
	var max int64
	for _, st := range q.Steps {
		if st.ID > max {
			max = st.ID
		}
	}
	return max, nil
}

func errText(code int, body []byte) string {
	var e struct {
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &e) == nil && e.Detail != "" {
		return fmt.Sprintf("%d: %s", code, e.Detail)
	}
	return fmt.Sprintf("%d: %s", code, strings.TrimSpace(string(body)))
}
