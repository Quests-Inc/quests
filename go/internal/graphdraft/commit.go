package graphdraft

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
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/refs"
	"github.com/quests-inc/quests/go/internal/store"
)

type draftProblem struct {
	Op      int64  `json:"op"`
	Message string `json:"message"`
}

func (h *Handler) commitGraphDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ops, err := h.Store.ListDraftOps(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(ops) == 0 {
		writeJSON(w, 200, map[string]any{"created": map[string]int64{}, "applied": 0})
		return
	}
	owners, problems := h.planDraft(ctx, ops)
	if len(problems) > 0 {
		writeJSON(w, 422, map[string]any{"error": "draft cannot be saved", "problems": problems})
		return
	}
	mux := h.API()
	created := map[string]int64{}
	var applied []int64
	fail := func(op store.DraftOp, err error) {
		// Keep what failed and what follows; swap placeholders that already
		// became real entities so a retry does not create them twice.
		_ = h.Store.DeleteDraftOps(ctx, applied)
		rest, _ := h.Store.ListDraftOps(ctx)
		for _, o := range rest {
			text := string(o.Payload)
			for k, id := range created {
				text = strings.ReplaceAll(text, `"`+k+`"`, `"`+strings.Split(k, ":")[0]+":"+strconv.FormatInt(id, 10)+`"`)
			}
			if text != string(o.Payload) {
				_ = h.Store.UpdateDraftOpPayload(ctx, o.ID, json.RawMessage(text))
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
			case "create_quest", "create_note", "create_questline":
				var p createPayload
				_ = json.Unmarshal(op.Payload, &p)
				path := "/api/quests"
				switch op.Kind {
				case "create_note":
					path = "/api/notes"
				case "create_questline":
					path = "/api/questlines"
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
				rc, gerr := h.WebDAV.Get(ctx, fp.Path)
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
				_ = h.WebDAV.Delete(context.WithoutCancel(ctx), fp.Path)
			case "copy":
				id, cerr := h.copyEntity(ctx, mux, op, owners[key], real, collection)
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
			if err := h.editRef(ctx, mux, op.Kind == "add_ref", ft, fid, fmt.Sprintf("%s=%d", tt, tid)); err != nil {
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
				qid, qerr := h.Store.Journal.QuestIDForStep(ctx, id)
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
		case "set_parent":
			var p parentPayload
			_ = json.Unmarshal(op.Payload, &p)
			_, id, ok := real(p.ID)
			if !ok {
				fail(op, errors.New("note missing"))
				return
			}
			var parent any
			if p.Parent != "" {
				_, pid, pok := real(p.Parent)
				if !pok {
					fail(op, errors.New("parent note missing"))
					return
				}
				parent = pid
			}
			body, _ := json.Marshal(map[string]any{"parent_id": parent})
			if code, out := dispatch(ctx, mux, "PATCH", fmt.Sprintf("/api/notes/%d", id), "application/json", body); code >= 300 {
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
				fromQ, qerr := h.Store.Journal.QuestIDForStep(ctx, aid)
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
	if err := h.Store.DeleteDraftOps(ctx, all); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"created": created, "applied": len(ops)})
}

// planDraft works out who owns each new step and file and reports what stops
// the draft from being saved.
func (h *Handler) planDraft(ctx context.Context, ops []store.DraftOp) (map[string]string, []draftProblem) {
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
			if qid, err := h.Store.Journal.QuestIDForStep(ctx, sid); err == nil {
				owners["step:"+store.TmpID(op.ID)] = fmt.Sprintf("quest:%d", qid)
			}
		case "attachment":
			if a, err := h.Store.Knowledge.GetAttachment(ctx, sid); err == nil {
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
		if origin, err := h.Store.Journal.QuestIDForStep(ctx, sid); err == nil {
			lastMoveOp[origin] = op.ID
		}
	}
	delta := map[int64]int{}
	for sid, dest := range finalOwner {
		origin, err := h.Store.Journal.QuestIDForStep(ctx, sid)
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
		q, err := h.Store.Journal.GetQuest(ctx, qid)
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
func (h *Handler) editRef(ctx context.Context, mux http.Handler, add bool, typ string, id int64, token string) error {
	var desc, path string
	switch typ {
	case "quest":
		q, err := h.Store.Journal.GetQuest(ctx, id)
		if err != nil {
			return err
		}
		desc, path = q.Description, fmt.Sprintf("/api/quests/%d", id)
	case "note":
		n, err := h.Store.Knowledge.GetNote(ctx, id)
		if err != nil {
			return err
		}
		desc, path = n.Description, fmt.Sprintf("/api/notes/%d", id)
	case "questline":
		ql, err := h.Store.Journal.GetQuestline(ctx, id)
		if err != nil {
			return err
		}
		desc = ql.Description
		path = fmt.Sprintf("/api/questlines/%d", id)
	case "step":
		qid, err := h.Store.Journal.QuestIDForStep(ctx, id)
		if err != nil {
			return err
		}
		q, err := h.Store.Journal.GetQuest(ctx, qid)
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
