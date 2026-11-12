package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/search"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestEntitySearchIdentityScopeAndPagination(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	note := doJSON(t, h, "POST", "/api/notes", map[string]any{"title": "Запись без номера", "description": "Подготовить развёртывание сервисов"}, nil)
	if note.Code != 201 {
		t.Fatal(note.Body.String())
	}
	qid := seedQuest(t, s)
	fetch := func(q search.Request, src string) search.Page {
		t.Helper()
		headers := map[string]string{}
		if src != "" {
			headers["X-Quests-Source"] = src
		}
		w := doJSON(t, h, "POST", "/api/search", q, headers)
		if w.Code != 200 {
			t.Fatalf("search %d: %s", w.Code, w.Body.String())
		}
		var p search.Page
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Every context projection is parsed, even if a type currently has no rows.
	page := fetch(search.Request{Types: search.Kinds, Limit: 100}, "")
	if len(page.Hits) < 2 {
		t.Fatalf("initial candidates: %+v", page)
	}
	page = fetch(search.Request{Query: fmt.Sprintf("quest=%d", qid), Types: []string{"quest", "note", "step"}}, "")
	if len(page.Hits) != 1 || page.Hits[0].Kind != "quest" || page.Hits[0].ID != qid || page.Hits[0].Where != "id" {
		t.Fatalf("typed identity: %+v", page)
	}
	if got := fetch(search.Request{Query: fmt.Sprint(qid), Types: []string{"quest"}, Allowed: map[string][]int64{"quest": {}}}, ""); len(got.Hits) != 0 {
		t.Fatalf("empty scope expanded: %+v", got)
	}
	if got := fetch(search.Request{Types: []string{}}, ""); len(got.Hits) != 0 {
		t.Fatalf("empty types expanded: %+v", got)
	}
	// The numeric query retains text matches, but the identity must precede them.
	_, err := s.Store.DB.ExecContext(t.Context(), `UPDATE quest SET title=? WHERE id=?`, fmt.Sprintf("Число %d внутри текста", qid), qid)
	if err != nil {
		t.Fatal(err)
	}
	page = fetch(search.Request{Query: fmt.Sprint(qid), Types: []string{"quest"}}, "")
	if len(page.Hits) == 0 || page.Hits[0].Where != "id" {
		t.Fatalf("numeric identity: %+v", page)
	}
	// All flag filtering happens before LIMIT, not after the first page of candidates.
	for i := 0; i < 65; i++ {
		w := doJSON(t, h, "POST", "/api/notes", map[string]any{"title": fmt.Sprintf("Результат %03d", i), "description": "текст", "pinned": i == 64}, nil)
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
	}
	page = fetch(search.Request{Query: "Результат", Types: []string{"note"}, Filters: []search.Filter{{Key: "flag", Values: []string{"pinned"}}}, Limit: 1}, "")
	if len(page.Hits) != 1 || page.Hits[0].Title != "Результат 064" {
		t.Fatalf("filter before limit: %+v", page)
	}
	first := fetch(search.Request{Query: "Результат", Types: []string{"note"}, Limit: 2}, "")
	second := fetch(search.Request{Query: "Результат", Types: []string{"note"}, Limit: 2, Offset: first.Next}, "")
	if !first.More || len(second.Hits) != 2 || first.Hits[0].ID == second.Hits[0].ID {
		t.Fatalf("pages: %+v / %+v", first, second)
	}
	mixed := fetch(search.Request{Types: []string{"quest", "note"}, Filters: []search.Filter{{Kind: "note", Key: "flag", Values: []string{"pinned"}}}, Limit: 100}, "")
	if len(mixed.Hits) != 2 {
		t.Fatalf("kind-specific filter excluded unrelated types: %+v", mixed)
	}
	// Privacy on the same path that web and MCP use.
	private := doJSON(t, h, "POST", "/api/notes", map[string]any{"title": "Скрытое слово", "description": "секрет", "is_private": true}, nil)
	if private.Code != 201 {
		t.Fatal(private.Body.String())
	}
	if got := fetch(search.Request{Query: "Скрытое", Types: []string{"note"}}, "mcp"); len(got.Hits) != 0 {
		t.Fatalf("private note leaked: %+v", got)
	}
	// RLS also applies to direct provider calls, including exact IDs and explicit scopes.
	hidden, err := s.Store.SearchEntities(wsctx.With(t.Context(), 999999), search.Request{Query: fmt.Sprintf("quest=%d", qid), Types: []string{"quest"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Hits) != 0 {
		t.Fatal("other-workspace identity leaked")
	}
	for _, body := range []any{map[string]any{"types": []string{"bogus"}}, map[string]any{"offset": -1}} {
		w := doJSON(t, h, http.MethodPost, "/api/search", body, nil)
		if w.Code != 422 {
			t.Fatalf("bad request: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestNoteCatalogKeepsBodiesOnDetailAndSearch(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	body := strings.Repeat("Описание. ", 100) + "уникальный маркер за пределами превью"
	w := doJSON(t, h, "POST", "/api/notes", map[string]any{"title": "Длинная заметка", "description": body}, nil)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var note map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	catalog := doJSON(t, h, "GET", "/api/notes?catalog=1", nil, nil)
	var rows []map[string]any
	if err := json.Unmarshal(catalog.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["description_complete"] != false || len([]rune(rows[0]["description"].(string))) > 320 {
		t.Fatalf("catalog: %s", catalog.Body.String())
	}
	detail := doJSON(t, h, "GET", fmt.Sprintf("/api/notes/%.0f", note["id"].(float64)), nil, nil)
	if !strings.Contains(detail.Body.String(), "уникальный") {
		t.Fatal("detail lost full body")
	}
	found := doJSON(t, h, "POST", "/api/search", search.Request{Query: "уникальный", Types: []string{"note"}}, nil)
	var page search.Page
	if err := json.Unmarshal(found.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 {
		t.Fatalf("search only used catalog preview: %s", found.Body.String())
	}
}
