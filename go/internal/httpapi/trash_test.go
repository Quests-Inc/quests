package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func apiCall(t *testing.T, h http.Handler, method, path string, body any) (int, string) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestQuestGoesToTheTrashAndComesBack(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	id := seedQuest(t, s)
	url := "/api/quests/" + strconv.FormatInt(id, 10)

	if code, _ := apiCall(t, h, http.MethodDelete, url, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodGet, url, nil); code != http.StatusNotFound {
		t.Fatalf("a trashed quest must be 404, got %d", code)
	}
	if _, list := apiCall(t, h, http.MethodGet, "/api/quests", nil); strings.Contains(list, `"id":`+strconv.FormatInt(id, 10)+`,`) {
		t.Fatalf("a trashed quest must not be listed: %s", list)
	}
	code, trash := apiCall(t, h, http.MethodGet, "/api/trash/quests", nil)
	if code != http.StatusOK || !strings.Contains(trash, `"id":`+strconv.FormatInt(id, 10)) || !strings.Contains(trash, `"kind":"quest"`) {
		t.Fatalf("trash list: %d %s", code, trash)
	}
	// Times carry the zone: a bare "2026-10-17 17:38:00" is read as local time by the browser (3 h off in Moscow).
	var items []struct {
		DeletedAt string `json:"deleted_at"`
		PurgeAt   string `json:"purge_at"`
	}
	if err := json.Unmarshal([]byte(trash), &items); err != nil || len(items) != 1 {
		t.Fatalf("trash json: %v %s", err, trash)
	}
	for _, v := range []string{items[0].DeletedAt, items[0].PurgeAt} {
		if _, err := time.Parse(time.RFC3339Nano, v); err != nil || !strings.HasSuffix(v, "Z") {
			t.Fatalf("a trash time must be RFC 3339 UTC (…Z), got %q (%v)", v, err)
		}
	}

	if code, body := apiCall(t, h, http.MethodPost, "/api/trash/quests/"+strconv.FormatInt(id, 10)+"/restore", nil); code != http.StatusOK {
		t.Fatalf("restore: %d %s", code, body)
	}
	if code, _ := apiCall(t, h, http.MethodGet, url, nil); code != http.StatusOK {
		t.Fatalf("a restored quest must be readable again, got %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodPost, "/api/trash/quests/"+strconv.FormatInt(id, 10)+"/restore", nil); code != http.StatusNotFound {
		t.Fatalf("restoring twice: %d", code)
	}
}

func TestPurgeAndEmptyTheTrash(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	a, b := seedQuest(t, s), seedQuest(t, s)
	for _, id := range []int64{a, b} {
		if code, _ := apiCall(t, h, http.MethodDelete, "/api/quests/"+strconv.FormatInt(id, 10), nil); code != http.StatusNoContent {
			t.Fatalf("delete %d: %d", id, code)
		}
	}
	if code, _ := apiCall(t, h, http.MethodDelete, "/api/trash/quests/"+strconv.FormatInt(a, 10), nil); code != http.StatusNoContent {
		t.Fatalf("purge one: %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodDelete, "/api/trash/quests/"+strconv.FormatInt(a, 10), nil); code != http.StatusNotFound {
		t.Fatalf("purging a quest that is gone: %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodDelete, "/api/trash/quests", nil); code != http.StatusNoContent {
		t.Fatalf("empty: %d", code)
	}
	if _, trash := apiCall(t, h, http.MethodGet, "/api/trash/quests", nil); strings.TrimSpace(trash) != "[]" {
		t.Fatalf("the trash must be empty: %s", trash)
	}
	// a live quest cannot be purged through the trash route
	live := seedQuest(t, s)
	if code, _ := apiCall(t, h, http.MethodDelete, "/api/trash/quests/"+strconv.FormatInt(live, 10), nil); code != http.StatusNotFound {
		t.Fatalf("a live quest is not in the trash: %d", code)
	}
}

func TestNoteTrashOverHTTP(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	code, body := apiCall(t, h, http.MethodPost, "/api/notes", map[string]any{"title": "n", "description": "d"})
	if code != http.StatusCreated {
		t.Fatalf("create note: %d %s", code, body)
	}
	var n struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &n)
	url := "/api/notes/" + strconv.FormatInt(n.ID, 10)

	if code, _ := apiCall(t, h, http.MethodDelete, url, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodGet, url, nil); code != http.StatusNotFound {
		t.Fatalf("a trashed note must be 404, got %d", code)
	}
	if code, tr := apiCall(t, h, http.MethodGet, "/api/trash/notes", nil); code != http.StatusOK || !strings.Contains(tr, `"kind":"note"`) {
		t.Fatalf("trash list: %d %s", code, tr)
	}
	if code, _ := apiCall(t, h, http.MethodPost, "/api/trash/notes/"+strconv.FormatInt(n.ID, 10)+"/restore", nil); code != http.StatusOK {
		t.Fatalf("restore: %d", code)
	}
	if code, _ := apiCall(t, h, http.MethodGet, url, nil); code != http.StatusOK {
		t.Fatalf("a restored note must be readable, got %d", code)
	}
	apiCall(t, h, http.MethodDelete, url, nil)
	if code, _ := apiCall(t, h, http.MethodDelete, "/api/trash/notes/"+strconv.FormatInt(n.ID, 10), nil); code != http.StatusNoContent {
		t.Fatalf("purge: %d", code)
	}
}

// backdate makes a trashed row look as if it had been deleted `age` ago.
func backdate(t *testing.T, s *Server, table string, id int64, age time.Duration) {
	t.Helper()
	ctx := wsctx.With(t.Context(), 1)
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.trash', '1', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET deleted_at = ? WHERE id = ?`, timeutil.ToDBUTC(timeutil.NowUTC().Add(-age)), id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestPurgeExpiredTrashErasesOnlyWhatIsOldEnough(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := wsctx.With(t.Context(), 1)
	oldQ, freshQ := seedQuest(t, s), seedQuest(t, s)
	oldN, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "old"})
	if err != nil {
		t.Fatal(err)
	}
	freshN, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "fresh"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{oldQ, freshQ} {
		if err := s.Store.Journal.TrashQuest(ctx, id, "q"); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{oldN.ID, freshN.ID} {
		if err := s.Store.Knowledge.TrashNote(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	backdate(t, s, "quest", oldQ, trashRetention+time.Hour)
	backdate(t, s, "note", oldN.ID, trashRetention+time.Hour)

	s.PurgeExpiredTrash(t.Context())

	quests, err := s.Store.Journal.ListTrashedQuests(ctx)
	if err != nil || len(quests) != 1 || quests[0].ID != freshQ {
		t.Fatalf("only the fresh quest may be left: %+v %v", quests, err)
	}
	notes, err := s.Store.Knowledge.ListTrashedNotes(ctx)
	if err != nil || len(notes) != 1 || notes[0].ID != freshN.ID {
		t.Fatalf("only the fresh note may be left: %+v %v", notes, err)
	}
}
