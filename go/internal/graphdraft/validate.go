package graphdraft

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/refs"
	"github.com/quests-inc/quests/go/internal/store"
)

// refExists tells whether a draft reference points at something real now or
// at an op already in the draft.
func (h *Handler) draftRefExists(ctx context.Context, ref draftRef, ops []store.DraftOp) bool {
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
			case "create_questline":
				return ref.Type == "questline"
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
		_, err := h.Store.Journal.GetQuest(ctx, id)
		return err == nil
	case "note":
		ok, err := h.Store.Knowledge.NoteExists(ctx, id)
		return err == nil && ok
	case "questline":
		_, err := h.Store.Journal.GetQuestline(ctx, id)
		return err == nil
	case "step":
		_, err := h.Store.Journal.QuestIDForStep(ctx, id)
		return err == nil
	case "attachment":
		_, err := h.Store.Knowledge.GetAttachment(ctx, id)
		return err == nil
	}
	return false
}

// validateDraftOp checks shape and that referenced things exist. Whether the
// draft as a whole can be saved is decided at commit.
func (h *Handler) validateDraftOp(ctx context.Context, kind string, raw json.RawMessage, ops []store.DraftOp) string {
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
		if !h.draftRefExists(ctx, ref, ops) {
			return what + ": not found"
		}
		return ""
	}
	switch kind {
	case "create_quest", "create_note", "create_step", "create_questline":
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
	case "set_parent":
		var p parentPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return "invalid payload"
		}
		if m := needRef(p.ID, "id", "note"); m != "" {
			return m
		}
		if p.Parent == "" {
			return ""
		}
		if p.Parent == p.ID {
			return "a note cannot be its own parent"
		}
		return needRef(p.Parent, "parent", "note")
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
		has := h.draftHasRef(ctx, p.From, p.To, ops)
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
func (h *Handler) draftHasRef(ctx context.Context, from, to string, ops []store.DraftOp) bool {
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
			if q, err := h.Store.Journal.GetQuest(ctx, id); err == nil {
				has = textHas(q.Description)
			}
		case "note":
			if n, err := h.Store.Knowledge.GetNote(ctx, id); err == nil {
				has = textHas(n.Description)
			}
		case "step":
			if qid, err := h.Store.Journal.QuestIDForStep(ctx, id); err == nil {
				if q, err := h.Store.Journal.GetQuest(ctx, qid); err == nil {
					for _, st := range q.Steps {
						if st.ID == id {
							has = textHas(st.Description)
						}
					}
				}
			}
		case "questline":
			if ql, err := h.Store.Journal.GetQuestline(ctx, id); err == nil {
				has = textHas(ql.Description)
			}
		}
	}
	for _, o := range ops {
		switch o.Kind {
		case "create_quest", "create_note", "create_step", "create_questline":
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
		Parent string `json:"parent"`
	}
	if json.Unmarshal(o.Payload, &p) != nil {
		return nil
	}
	var out []string
	for _, v := range []string{p.From, p.To, p.Item, p.Owner, p.Quest, p.ID, p.Source, p.Parent} {
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
	case "create_questline":
		return "questline:" + store.TmpID(o.ID)
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
