package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
)

func TestAttachmentPreviewRevisionAndCache(t *testing.T) {
	dav := newFakeDAV()
	defer dav.Close()
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	qid := seedQuest(t, s)
	h := s.Handler()
	base := fmt.Sprintf("/api/quests/%d/attachments", qid)
	w := postFile(t, h, base, "example.html", "", []byte("<h1>First revision</h1>"))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a knowledge.AttachmentRead
	if e := json.Unmarshal(w.Body.Bytes(), &a); e != nil {
		t.Fatal(e)
	}
	path := fmt.Sprintf("/api/attachments/%d/preview", a.ID)
	get := func(path, tag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if tag != "" {
			req.Header.Set("If-None-Match", tag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	first := get(path, "")
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	var preview struct{ Kind, Text string }
	if e := json.Unmarshal(first.Body.Bytes(), &preview); e != nil {
		t.Fatal(e)
	}
	if preview.Text != "<h1>First revision</h1>" || preview.Kind != "text" {
		t.Fatalf("%+v", preview)
	}
	if first.Header().Get("Content-Type") != "application/json" {
		t.Fatal(first.Header())
	}
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag")
	}
	if rec := get(path, tag); rec.Code != 304 {
		t.Fatal(rec.Code)
	}
	rev := postFile(t, h, fmt.Sprintf("%s/%d/revisions", base, a.ID), "example.html", "", []byte("Second revision"))
	if rev.Code != 201 {
		t.Fatal(rev.Code, rev.Body.String())
	}
	latest := get(path, tag)
	if latest.Code != 200 || latest.Header().Get("ETag") == tag {
		t.Fatal(latest.Code, latest.Header())
	}
	old := get(path+"?revision=1", "")
	if e := json.Unmarshal(old.Body.Bytes(), &preview); e != nil {
		t.Fatal(e)
	}
	if preview.Text != "<h1>First revision</h1>" {
		t.Fatalf("old revision lost: %s", old.Body.String())
	}
	if rec := get(path+"?revision=no", ""); rec.Code != 422 {
		t.Fatal(rec.Code)
	}
	if rec := get("/api/attachments/999999/preview", ""); rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
