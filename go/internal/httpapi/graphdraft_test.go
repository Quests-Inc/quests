package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valirum/quests/go/internal/store"
)

func draftReq(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = strings.NewReader(string(b))
	} else {
		rdr = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, rdr)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func addOp(t *testing.T, h http.Handler, kind string, payload any) int64 {
	t.Helper()
	code, out := draftReq(t, h, "POST", "/api/graph-draft/ops", map[string]any{"kind": kind, "payload": payload})
	if code != 201 {
		t.Fatalf("add %s = %d %v", kind, code, out)
	}
	return int64(out["id"].(float64))
}

func TestGraphDraftCommitFlow(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	h := s.Handler()

	// An existing quest with a file that will be moved to a new note.
	oldQ := seedQuest(t, s)
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", oldQ), "old.md", "", []byte("# old"))
	if up.Code != 201 {
		t.Fatalf("seed upload = %d %s", up.Code, up.Body.String())
	}
	var oldAtt map[string]any
	_ = json.Unmarshal(up.Body.Bytes(), &oldAtt)
	oldAttID := int64(oldAtt["id"].(float64))

	note := addOp(t, h, "create_note", map[string]any{"fields": map[string]any{"title": "Новая заметка"}})
	quest := addOp(t, h, "create_quest", map[string]any{"fields": map[string]any{"title": "Новый квест"}})
	tn, tq := fmt.Sprintf("note:t%d", note), fmt.Sprintf("quest:t%d", quest)
	step := addOp(t, h, "create_step", map[string]any{"fields": map[string]any{"title": "Шаг"}})
	ts := fmt.Sprintf("step:t%d", step)

	// A step with no quest blocks saving.
	code, out := draftReq(t, h, "POST", "/api/graph-draft/commit", nil)
	if code != 422 || out["problems"] == nil {
		t.Fatalf("commit must refuse an orphan step: %d %v", code, out)
	}
	addOp(t, h, "attach", map[string]any{"item": ts, "owner": tq})
	addOp(t, h, "add_ref", map[string]any{"from": tq, "to": tn})
	addOp(t, h, "add_ref", map[string]any{"from": tq, "to": fmt.Sprintf("quest:%d", oldQ)})
	addOp(t, h, "attach", map[string]any{"item": fmt.Sprintf("attachment:%d", oldAttID), "owner": tn})

	// A staged file attached to the new note.
	fileOp := func() int64 {
		var buf strings.Builder
		_ = buf
		w := postFile(t, h, "/api/graph-draft/files", "draft.txt", "", []byte("hello draft"))
		if w.Code != 201 {
			t.Fatalf("stage file = %d %s", w.Code, w.Body.String())
		}
		var o map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &o)
		return int64(o["id"].(float64))
	}()
	addOp(t, h, "attach", map[string]any{"item": fmt.Sprintf("attachment:t%d", fileOp), "owner": tn})

	code, out = draftReq(t, h, "POST", "/api/graph-draft/commit", nil)
	if code != 200 {
		t.Fatalf("commit = %d %v", code, out)
	}
	created := out["created"].(map[string]any)
	noteID := int64(created[tn].(float64))
	questID := int64(created[tq].(float64))

	q, err := s.Store.GetQuest(t.Context(), questID)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Steps) == 0 || q.Steps[len(q.Steps)-1].Title != "Шаг" {
		t.Fatalf("step missing: %+v", q.Steps)
	}
	if !strings.Contains(q.Description, fmt.Sprintf("note=%d", noteID)) || !strings.Contains(q.Description, fmt.Sprintf("quest=%d", oldQ)) {
		t.Fatalf("refs not appended: %q", q.Description)
	}
	atts, _ := s.Store.ListAttachments(t.Context(), "note", noteID)
	if len(atts) != 2 {
		t.Fatalf("note attachments = %d, want the moved file and the staged one", len(atts))
	}
	moved, _ := s.Store.GetAttachment(t.Context(), oldAttID)
	if moved.OwnerType != "note" || moved.OwnerID != noteID {
		t.Fatalf("attachment not re-parented: %+v", moved)
	}
	if left, _ := s.Store.ListDraftOps(t.Context()); len(left) != 0 {
		t.Fatalf("draft not cleared: %d ops", len(left))
	}
}

func TestGraphDraftRemoveRefAndCascade(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	h := s.Handler()
	qid := seedQuest(t, s)
	if _, err := s.Store.DB.Exec(`UPDATE quest SET description = ? WHERE id = ?`, "see note=5 and note=55 here", qid); err != nil {
		t.Fatal(err)
	}
	// Only the token edit matters here, so the helper is used directly.
	if got := editToken("see note=5 and note=55 here", "note=5", false); !strings.Contains(got, "note=55") || strings.Contains(got, "note=5 ") {
		t.Fatalf("remove touched the wrong token: %q", got)
	}
	if got := editToken("text", "note=7", true); got != "text\n\nnote=7" {
		t.Fatalf("append = %q", got)
	}
	if got := editToken("text\n\nnote=7", "note=7", true); got != "text\n\nnote=7" {
		t.Fatalf("append must be idempotent: %q", got)
	}

	// Dropping a created thing drops what depends on it.
	n := addOp(t, h, "create_note", map[string]any{"fields": map[string]any{"title": "tmp"}})
	link := addOp(t, h, "add_ref", map[string]any{"from": fmt.Sprintf("quest:%d", qid), "to": fmt.Sprintf("note:t%d", n)})
	code, out := draftReq(t, h, "DELETE", fmt.Sprintf("/api/graph-draft/ops/%d?dry=1", n), nil)
	if code != 200 || len(out["would_remove"].([]any)) != 2 {
		t.Fatalf("dry run = %d %v", code, out)
	}
	code, out = draftReq(t, h, "DELETE", fmt.Sprintf("/api/graph-draft/ops/%d", n), nil)
	if code != 200 {
		t.Fatalf("delete = %d %v", code, out)
	}
	ops, _ := s.Store.ListDraftOps(t.Context())
	if len(ops) != 0 {
		t.Fatalf("ops left after cascade: %d (link op %d)", len(ops), link)
	}
	// Cancel drops everything and the staged file.
	w := postFile(t, h, "/api/graph-draft/files", "x.txt", "", []byte("x"))
	if w.Code != 201 {
		t.Fatalf("stage = %d", w.Code)
	}
	if code, _ := draftReq(t, h, "DELETE", "/api/graph-draft", nil); code != 204 {
		t.Fatalf("cancel = %d", code)
	}
	if ops, _ := s.Store.ListDraftOps(t.Context()); len(ops) != 0 {
		t.Fatalf("cancel left %d ops", len(ops))
	}
}

func TestGraphDraftRefusesDuplicateLinks(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	h := s.Handler()
	a := seedQuest(t, s)
	b := seedQuest(t, s)
	from, to := fmt.Sprintf("quest:%d", a), fmt.Sprintf("quest:%d", b)

	post := func(kind string) int {
		code, _ := draftReq(t, h, "POST", "/api/graph-draft/ops", map[string]any{"kind": kind, "payload": map[string]any{"from": from, "to": to}})
		return code
	}
	if code := post("remove_ref"); code != 422 {
		t.Fatalf("removing a link that is not there = %d, want 422", code)
	}
	if code := post("add_ref"); code != 201 {
		t.Fatalf("first add = %d", code)
	}
	if code := post("add_ref"); code != 422 {
		t.Fatalf("second add in the draft = %d, want 422", code)
	}
	// Already in the saved text.
	if _, err := s.Store.DB.Exec(`UPDATE quest SET description = ? WHERE id = ?`, fmt.Sprintf("see quest=%d", a), b); err != nil {
		t.Fatal(err)
	}
	code, _ := draftReq(t, h, "POST", "/api/graph-draft/ops", map[string]any{"kind": "add_ref", "payload": map[string]any{"from": to, "to": from}})
	if code != 422 {
		t.Fatalf("add over an existing link = %d, want 422", code)
	}
	// Removing it in the draft makes room to add it again.
	if c, _ := draftReq(t, h, "POST", "/api/graph-draft/ops", map[string]any{"kind": "remove_ref", "payload": map[string]any{"from": to, "to": from}}); c != 201 {
		t.Fatalf("remove saved link = %d", c)
	}
	if c, _ := draftReq(t, h, "POST", "/api/graph-draft/ops", map[string]any{"kind": "add_ref", "payload": map[string]any{"from": to, "to": from}}); c != 201 {
		t.Fatalf("re-add after remove = %d", c)
	}
}

func TestMoveExistingStep(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	h := s.Handler()
	mk := func(title string, steps ...string) int64 {
		var st []map[string]any
		for _, x := range steps {
			st = append(st, map[string]any{"title": x})
		}
		code, out := draftReq(t, h, "POST", "/api/quests", map[string]any{"title": title, "steps": st})
		if code != 201 {
			t.Fatalf("create quest = %d %v", code, out)
		}
		return int64(out["id"].(float64))
	}
	a := mk("A", "a1", "a2")
	b := mk("B", "b1")
	qa, _ := s.Store.GetQuest(t.Context(), a)
	moveID := qa.Steps[0].ID

	// Through the endpoint.
	code, out := draftReq(t, h, "POST", fmt.Sprintf("/api/quests/%d/steps/%d/move", a, moveID), map[string]any{"quest_id": b})
	if code != 200 {
		t.Fatalf("move = %d %v", code, out)
	}
	qa, _ = s.Store.GetQuest(t.Context(), a)
	qb, _ := s.Store.GetQuest(t.Context(), b)
	if len(qa.Steps) != 1 || len(qb.Steps) != 2 || qb.Steps[1].ID != moveID {
		t.Fatalf("after move: A=%d steps, B=%d steps (%+v)", len(qa.Steps), len(qb.Steps), qb.Steps)
	}
	// The last step of a quest cannot leave; moving into the same quest is refused.
	if code, _ := draftReq(t, h, "POST", fmt.Sprintf("/api/quests/%d/steps/%d/move", a, qa.Steps[0].ID), map[string]any{"quest_id": b}); code != 422 {
		t.Fatalf("moving the last step = %d, want 422", code)
	}
	if code, _ := draftReq(t, h, "POST", fmt.Sprintf("/api/quests/%d/steps/%d/move", b, moveID), map[string]any{"quest_id": b}); code != 422 {
		t.Fatalf("moving into the same quest = %d, want 422", code)
	}

	// Through the draft: move it back, and see the guard on commit.
	op := addOp(t, h, "attach", map[string]any{"item": fmt.Sprintf("step:%d", moveID), "owner": fmt.Sprintf("quest:%d", a)})
	if code, out := draftReq(t, h, "POST", "/api/graph-draft/commit", nil); code != 200 {
		t.Fatalf("commit = %d %v", code, out)
	}
	qa, _ = s.Store.GetQuest(t.Context(), a)
	if len(qa.Steps) != 2 {
		t.Fatalf("step did not come back: %d", len(qa.Steps))
	}
	_ = op
	// Taking a quest's last step is fine in the draft, but saving is refused
	// while the quest would be left with none ...
	qb, _ = s.Store.GetQuest(t.Context(), b)
	b1 := qb.Steps[0].ID
	take := addOp(t, h, "attach", map[string]any{"item": fmt.Sprintf("step:%d", b1), "owner": fmt.Sprintf("quest:%d", a)})
	code, out = draftReq(t, h, "POST", "/api/graph-draft/commit", nil)
	if code != 422 || out["problems"] == nil {
		t.Fatalf("draining a quest must be refused: %d %v", code, out)
	}
	// ... and allowed once another step is brought in.
	fresh := addOp(t, h, "create_step", map[string]any{"fields": map[string]any{"title": "замена"}, "quest": fmt.Sprintf("quest:%d", b)})
	_ = fresh
	if code, out := draftReq(t, h, "POST", "/api/graph-draft/commit", nil); code != 200 {
		t.Fatalf("commit with a replacement step = %d %v", code, out)
	}
	_ = take
	qb, _ = s.Store.GetQuest(t.Context(), b)
	if len(qb.Steps) != 1 || qb.Steps[0].Title != "замена" {
		t.Fatalf("B should hold only the replacement: %+v", qb.Steps)
	}
	qa, _ = s.Store.GetQuest(t.Context(), a)
	if len(qa.Steps) != 3 {
		t.Fatalf("A should have gained b1: %d steps", len(qa.Steps))
	}
}

func TestDraftCascadeRules(t *testing.T) {
	mk := func(id int64, kind, payload string) store.DraftOp {
		return store.DraftOp{ID: id, Kind: kind, Payload: json.RawMessage(payload)}
	}
	ids := func(in []int64) string { return fmt.Sprint(in) }

	// A made thing takes what uses it, transitively; unrelated ops stay.
	ops := []store.DraftOp{
		mk(1, "create_quest", `{"fields":{"title":"A"}}`),
		mk(2, "create_step", `{"fields":{"title":"s"},"quest":"quest:t1"}`),
		mk(3, "add_ref", `{"from":"step:t2","to":"note:5"}`),
		mk(4, "add_ref", `{"from":"quest:9","to":"note:5"}`),
		mk(12, "create_note", `{"fields":{"title":"N"}}`),
		mk(13, "add_ref", `{"from":"quest:9","to":"note:t12"}`),
	}
	if got := ids(dependents(ops, 1)); got != "[1 2 3]" {
		t.Fatalf("cascade from op 1 = %s, want [1 2 3]", got)
	}
	// Op 1 must not be confused with op 12 (t1 vs t12).
	if got := ids(dependents(ops, 12)); got != "[12 13]" {
		t.Fatalf("cascade from op 12 = %s, want [12 13]", got)
	}
	// Free text that looks like a placeholder is not a reference.
	text := []store.DraftOp{
		mk(1, "create_quest", `{"fields":{"title":"A"}}`),
		mk(2, "create_note", `{"fields":{"title":"n","description":"see quest:t1"}}`),
	}
	if got := ids(dependents(text, 1)); got != "[1]" {
		t.Fatalf("text must not count as a reference: %s", got)
	}

	// Add / remove of the same link: dropping the first takes the next
	// opposite op with it, but not what follows.
	pair := []store.DraftOp{
		mk(1, "add_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(2, "remove_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(3, "add_ref", `{"from":"quest:1","to":"note:2"}`),
		mk(4, "add_ref", `{"from":"quest:1","to":"note:3"}`),
	}
	if got := ids(dependents(pair, 1)); got != "[1 2]" {
		t.Fatalf("dropping an add must drop the next remove only: %s", got)
	}
	if got := ids(dependents(pair, 3)); got != "[3]" {
		t.Fatalf("the last op of a pair depends on nothing after it: %s", got)
	}
	if got := ids(dependents(pair, 4)); got != "[4]" {
		t.Fatalf("an unrelated link stays alone: %s", got)
	}
}
