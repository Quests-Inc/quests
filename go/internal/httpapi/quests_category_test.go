package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/timeutil"
)

// TestPatchQuestCategoryFollowsQuestline covers the drift bug: manually
// PATCHing category_id on a quest that already belongs to a questline with
// its own category must not stick — the questline's category always wins.
func TestPatchQuestCategoryFollowsQuestline(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	// Seed two categories and a questline pinned to the first one.
	var workCat, funCat int64
	for _, cat := range []struct {
		id   *int64
		slug string
	}{{&workCat, "work"}, {&funCat, "fun"}} {
		res, err := s.Store.DB.Exec(
			`INSERT INTO questcategory (slug, label, color, created_at) VALUES (?, ?, '#000000', now())`,
			cat.slug, cat.slug,
		)
		if err != nil {
			t.Fatalf("insert category: %v", err)
		}
		id, _ := res.LastInsertId()
		*cat.id = id
	}
	res, err := s.Store.DB.Exec(
		`INSERT INTO questline (title, category_id, created_at, updated_at) VALUES ('line', ?, now(), now())`,
		workCat,
	)
	if err != nil {
		t.Fatalf("insert questline: %v", err)
	}
	lineID, _ := res.LastInsertId()

	qid := seedQuest(t, s)

	// Assign the quest to the questline first (no category_id in this call).
	patch(t, h, qid, map[string]any{"questline_id": lineID})
	if got := questCategory(t, h, qid); got != workCat {
		t.Fatalf("after questline assign: category = %d, want %d", got, workCat)
	}

	// Now try to manually override the category on a quest that's already in
	// the questline — this is the bug: it used to stick.
	patch(t, h, qid, map[string]any{"category_id": funCat})
	if got := questCategory(t, h, qid); got != workCat {
		t.Fatalf("after manual category override: category = %d, want questline's %d (got %d)", workCat, workCat, got)
	}
}

func patch(t *testing.T, h http.Handler, qid int64, body map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/quests/"+strconv.FormatInt(qid, 10), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch quest %d: status %d: %s", qid, w.Code, w.Body.String())
	}
}

func questCategory(t *testing.T, h http.Handler, qid int64) int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/quests/"+strconv.FormatInt(qid, 10), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get quest %d: status %d: %s", qid, w.Code, w.Body.String())
	}
	var out struct {
		CategoryID *int64 `json:"category_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode quest: %v", err)
	}
	if out.CategoryID == nil {
		t.Fatalf("quest %d has no category_id", qid)
	}
	return *out.CategoryID
}

// TestGetQuestResolvesRefs covers the parity gap fixed alongside quest=214:
// getQuest didn't resolve note=/attachment=/etc. mentions in its
// description the way getNote already did for notes.
func TestGetQuestResolvesRefs(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	res, err := s.Store.DB.Exec(
		`INSERT INTO note (title, description, created_at, updated_at) VALUES ('Toolkit', '', now(), now())`,
	)
	if err != nil {
		t.Fatalf("insert note: %v", err)
	}
	noteID, _ := res.LastInsertId()

	now := timeutil.NowUTC()
	q, err := s.Store.CreateQuest(t.Context(), domain.Quest{
		Title:        "refs test",
		Description:  fmt.Sprintf("see note=%d", noteID),
		Status:       domain.StatusActive,
		Significance: domain.SigCommon,
		CreatedAt:    now,
		UpdatedAt:    now,
		Steps:        []domain.Step{{Title: "one", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatalf("create quest: %v", err)
	}

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/quests/%d", q.ID), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var out struct {
		Refs []map[string]any `json:"refs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Refs) != 1 {
		t.Fatalf("refs = %+v, want exactly one resolved note ref", out.Refs)
	}
	if out.Refs[0]["kind"] != "note" || int64(out.Refs[0]["id"].(float64)) != noteID {
		t.Fatalf("refs[0] = %+v, want kind=note id=%d", out.Refs[0], noteID)
	}
	if out.Refs[0]["title"] != "Toolkit" {
		t.Fatalf("refs[0].title = %v, want resolved note title", out.Refs[0]["title"])
	}
}
