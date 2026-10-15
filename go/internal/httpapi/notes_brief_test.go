package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
)

func TestNotesBriefAndCategoryFilter(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	create := func(body map[string]any) int64 {
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
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return int64(out["id"].(float64))
	}
	get := func(url string) (int, []byte) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		return w.Code, w.Body.Bytes()
	}

	long := strings.Repeat("я", 3000)
	cat := create(map[string]any{"title": "Категория", "description": "что сюда класть", "is_category": true})
	child := create(map[string]any{"title": "Ребёнок", "description": long, "parent_id": cat})
	grand := create(map[string]any{"title": "Внук", "description": long, "parent_id": child})
	outside := create(map[string]any{"title": "Снаружи", "description": long, "no_category": true})
	_ = outside

	code, body := get("/api/notes?brief=1&category_id=" + itoa(cat))
	if code != http.StatusOK {
		t.Fatalf("status %d %s", code, body)
	}
	if len(body) > 1500 {
		t.Fatalf("brief page must not carry text, %d bytes", len(body))
	}
	var page knowledgehttp.NoteBriefPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("category subtree must hold child+grandchild: %+v", page)
	}
	got := map[int64]int{}
	for _, it := range page.Items {
		got[it.ID] = it.DescriptionLen
	}
	if got[child] != 3000 || got[grand] != 3000 {
		t.Fatalf("description_len %v", got)
	}

	// Paging over the whole vault keeps order and covers each note once.
	seen := map[int64]bool{}
	cursor := ""
	for i := 0; i < 10; i++ {
		_, b := get("/api/notes?brief=1&limit=2&cursor=" + cursor)
		var p knowledgehttp.NoteBriefPage
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		for _, it := range p.Items {
			if seen[it.ID] {
				t.Fatalf("note %d twice", it.ID)
			}
			seen[it.ID] = true
		}
		if !p.Truncated {
			break
		}
		cursor = p.Next
	}
	if len(seen) != 4 {
		t.Fatalf("walk saw %d notes want 4", len(seen))
	}

	if code, _ := get("/api/notes?category_id=" + itoa(child)); code != http.StatusUnprocessableEntity {
		t.Fatalf("non-category: %d", code)
	}
	if code, _ := get("/api/notes?category_id=999999"); code != http.StatusNotFound {
		t.Fatalf("missing category: %d", code)
	}
	if code, _ := get("/api/notes?brief=1&cursor=999999"); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", code)
	}

	// Without brief: the full array, text included, now filtered by category on the server.
	code, body = get("/api/notes?category_id=" + itoa(cat))
	var full []map[string]any
	if err := json.Unmarshal(body, &full); code != http.StatusOK || err != nil || len(full) != 2 {
		t.Fatalf("legacy list %d %v len=%d", code, err, len(full))
	}
	if full[0]["description"] == "" {
		t.Fatal("legacy list keeps text")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
