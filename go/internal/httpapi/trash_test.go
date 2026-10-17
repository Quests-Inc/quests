package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
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
