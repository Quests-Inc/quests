package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoteCRUDAndContextLinks(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	create := func(body map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	person := create(map[string]any{"title": "Титульник", "description": "Иванов И.И., группа 1"})
	personID := int64(person["id"].(float64))
	toolkit := create(map[string]any{
		"title":       "ГОСТ",
		"description": "Инструкция. Данные: note=1. Пример: quest=99",
	})
	toolkitID := int64(toolkit["id"].(float64))
	if toolkitID == personID {
		t.Fatal("expected distinct notes")
	}

	qid := seedQuest(t, s)
	_, err := s.Store.DB.ExecContext(t.Context(),
		`UPDATE quest SET description=? WHERE id=?`,
		"написать реферат, тулкит note="+fmt.Sprint(toolkitID), qid)
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/context?note=%d", toolkitID), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("context note %d %s", w.Code, w.Body.String())
	}
	var ctx map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &ctx); err != nil {
		t.Fatal(err)
	}
	note, _ := ctx["note"].(map[string]any)
	if note == nil {
		t.Fatalf("missing note: %v", ctx)
	}
	refs, _ := note["refs"].([]any)
	if len(refs) < 1 {
		t.Fatalf("expected outbound refs, got %v", note["refs"])
	}

	r = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/context?quest=%d", qid), nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("context quest %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ctx); err != nil {
		t.Fatal(err)
	}
	linked, _ := ctx["linked_notes"].([]any)
	if len(linked) != 1 {
		t.Fatalf("linked_notes=%v", ctx["linked_notes"])
	}

	r = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/notes/%d", personID), nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("get person %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &person); err != nil {
		t.Fatal(err)
	}
	backs, _ := person["backlinks"].([]any)
	if len(backs) != 1 {
		t.Fatalf("backlinks to person=%v", person["backlinks"])
	}

	// Parent cycle
	raw, _ := json.Marshal(map[string]any{"parent_id": personID})
	r = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/notes/%d", personID), bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 422 {
		t.Fatalf("self-parent want 422 got %d %s", w.Code, w.Body.String())
	}
}

func TestNoteDescriptionInsertAndReplace(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	raw, _ := json.Marshal(map[string]any{
		"title":       "partial",
		"description": "AAA BBB AAA",
	})
	r := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var note map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	id := int64(note["id"].(float64))

	patch := func(body map[string]any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/notes/%d", id), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, out := patch(map[string]any{
		"description_insert": map[string]any{"text": "!", "after": "BBB"},
	})
	if code != 200 || out["description"] != "AAA BBB! AAA" {
		t.Fatalf("insert after: %d %v", code, out)
	}

	code, out = patch(map[string]any{
		"description_replace": map[string]any{"find": "AAA", "with": "X", "all": false},
	})
	if code != 200 || out["description"] != "X BBB! AAA" {
		t.Fatalf("replace first: %d %v", code, out)
	}

	code, out = patch(map[string]any{
		"description_replace": map[string]any{"find": "нет", "with": "Y"},
	})
	if code != 422 {
		t.Fatalf("missing find want 422 got %d %v", code, out)
	}
	// The error body carries no note; re-get it to check the note itself is unchanged.
	r = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/notes/%d", id), nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	_ = json.Unmarshal(w.Body.Bytes(), &note)
	if note["description"] != "X BBB! AAA" {
		t.Fatalf("note wiped on miss: %v", note["description"])
	}

	code, _ = patch(map[string]any{
		"description":        "full",
		"description_insert": map[string]any{"text": "x"},
	})
	if code != 422 {
		t.Fatalf("mix full+insert want 422 got %d", code)
	}
}

// A private note hides its description from MCP (X-Quests-Source: mcp), but
// every other source still sees it — see note=82 for the design.
func TestPrivateNoteHidesDescriptionOnlyFromMCP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	post := func(path, source string, body map[string]any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if source != "" {
			r.Header.Set("X-Quests-Source", source)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	get := func(path, source string) map[string]any {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if source != "" {
			r.Header.Set("X-Quests-Source", source)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("get %d %s", w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	n := post("/api/notes", "web", map[string]any{
		"title": "Пароль", "description": "sk-super-secret", "is_private": true,
	})
	id := int64(n["id"].(float64))
	if n["description"] != "sk-super-secret" {
		t.Fatalf("own create response (web) must see real content, got %v", n["description"])
	}

	viaMCP := get(fmt.Sprintf("/api/notes/%d", id), "mcp")
	if d, _ := viaMCP["description"].(string); d == "sk-super-secret" || d == "" {
		t.Fatalf("mcp get must not see the real description, got %q", d)
	}
	if viaMCP["title"] != "Пароль" {
		t.Fatalf("mcp get must still see the title, got %v", viaMCP["title"])
	}

	viaWeb := get(fmt.Sprintf("/api/notes/%d", id), "web")
	if viaWeb["description"] != "sk-super-secret" {
		t.Fatalf("web get must see the real description, got %v", viaWeb["description"])
	}

	list := []map[string]any{}
	r := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	r.Header.Set("X-Quests-Source", "mcp")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if int64(row["id"].(float64)) == id {
			if row["description"] == "sk-super-secret" {
				t.Fatal("list_notes via mcp must not leak the private description")
			}
		}
	}

	// A non-private note is unaffected.
	open := post("/api/notes", "web", map[string]any{"title": "Обычная", "description": "видно всем"})
	openID := int64(open["id"].(float64))
	if g := get(fmt.Sprintf("/api/notes/%d", openID), "mcp"); g["description"] != "видно всем" {
		t.Fatalf("non-private note must be visible via mcp too, got %v", g["description"])
	}
}

// Regression: get_note_context (GET /api/context?note=N) used the raw
// notePayload, bypassing the private-note redaction that list/get already
// had — found 2026-10-02 when a note marked private right after this feature
// shipped still leaked its description through this path.
func TestPrivateNoteHidesDescriptionViaContextEndpointToo(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	raw, _ := json.Marshal(map[string]any{
		"title": "Пароль", "description": "sk-super-secret", "is_private": true,
	})
	r := httptest.NewRequest(http.MethodPost, "/api/notes", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := int64(created["id"].(float64))

	for _, path := range []string{
		fmt.Sprintf("/api/context?note=%d", id),
	} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Quests-Source", "mcp")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		note, ok := out["note"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no note object in response: %v", path, out)
		}
		if note["description"] == "sk-super-secret" {
			t.Fatalf("%s: private description leaked via get_note_context: %v", path, note["description"])
		}
	}
}
