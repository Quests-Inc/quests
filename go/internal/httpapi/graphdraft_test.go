package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
