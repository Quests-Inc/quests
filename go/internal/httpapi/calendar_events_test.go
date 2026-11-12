package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func postEventIcon(t *testing.T, h http.Handler, id int64, name string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/calendar/events/%d/icon", id), &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCalendarEventCRUDAndIcon(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()

	createBody, _ := json.Marshal(map[string]any{
		"title": "День", "description": "", "starts_at": "2027-01-01T00:00:00Z",
		"ends_at": "2027-01-02T00:00:00Z", "all_day": true, "color": "#c47a20", "icon": "flag",
	})
	cw := httptest.NewRecorder()
	h.ServeHTTP(cw, httptest.NewRequest(http.MethodPost, "/api/calendar/events", bytes.NewReader(createBody)))
	if cw.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", cw.Code, cw.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(cw.Body.Bytes(), &created)
	id := int64(created["id"].(float64))

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/calendar/events", nil))
	if list.Code != 200 {
		t.Fatalf("list = %d", list.Code)
	}

	qid := seedQuest(t, s)
	link, _ := json.Marshal(map[string]any{
		"event_id": id, "event_edge": "start", "event_dir": "before", "event_offset_seconds": 3600,
	})
	pw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/quests/"+strconv.FormatInt(qid, 10), bytes.NewReader(link))
	h.ServeHTTP(pw, req)
	if pw.Code != 200 {
		t.Fatalf("link quest = %d %s", pw.Code, pw.Body.String())
	}

	patchBody, _ := json.Marshal(map[string]any{
		"title": "День 2", "description": "x", "starts_at": "2027-01-01T00:00:00Z",
		"ends_at": "2027-01-03T00:00:00Z", "all_day": true, "color": "#5a8a9a", "icon": "calendar",
	})
	uw := httptest.NewRecorder()
	h.ServeHTTP(uw, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/calendar/events/%d", id), bytes.NewReader(patchBody)))
	if uw.Code != 200 {
		t.Fatalf("patch = %d %s", uw.Code, uw.Body.String())
	}

	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	iw := postEventIcon(t, h, id, "mark.png", png)
	if iw.Code != 200 {
		t.Fatalf("upload icon = %d %s", iw.Code, iw.Body.String())
	}
	var withIcon map[string]any
	_ = json.Unmarshal(iw.Body.Bytes(), &withIcon)
	if withIcon["icon_url"] == nil || withIcon["icon_url"] == "" {
		t.Fatalf("icon_url missing: %v", withIcon)
	}

	gw := httptest.NewRecorder()
	h.ServeHTTP(gw, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/calendar/events/%d/icon", id), nil))
	if gw.Code != 200 {
		t.Fatalf("get icon = %d", gw.Code)
	}
	got, _ := io.ReadAll(gw.Body)
	if len(got) == 0 {
		t.Fatal("empty icon body")
	}

	dw := httptest.NewRecorder()
	h.ServeHTTP(dw, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/calendar/events/%d/icon", id), nil))
	if dw.Code != 200 {
		t.Fatalf("clear icon = %d %s", dw.Code, dw.Body.String())
	}

	del := httptest.NewRecorder()
	h.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/calendar/events/%d", id), nil))
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", del.Code, del.Body.String())
	}
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/calendar/events/%d", id), nil))
	if missing.Code != 404 {
		t.Fatalf("get after delete = %d", missing.Code)
	}
}
