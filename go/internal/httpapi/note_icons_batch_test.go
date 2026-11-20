package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNoteIconBatchCopiesOneFileAndRejectsTheWholeList(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	s.DataDir = t.TempDir()
	h := s.Handler()
	create := func(title string) int64 {
		t.Helper()
		w := doJSON(t, h, http.MethodPost, "/api/notes", map[string]any{"title": title}, nil)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %d %s", w.Code, w.Body.String())
		}
		var note map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &note); err != nil {
			t.Fatal(err)
		}
		return int64(note["id"].(float64))
	}
	source := create("Источник")
	left := create("Левая")
	right := create("Правая")
	raw := []byte("png-bytes-for-one-upload")
	up := postFile(t, h, "/api/notes/"+strconv.FormatInt(source, 10)+"/icon", "mark.png", "", raw)
	if up.Code != http.StatusOK {
		t.Fatalf("upload %d %s", up.Code, up.Body.String())
	}
	body := map[string]any{"ids": []int64{left, right}, "color": "#c47a20", "icon": "flag", "copy_from": source}
	got := doJSON(t, h, http.MethodPost, "/api/notes/icons", body, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("apply %d %s", got.Code, got.Body.String())
	}
	var applied struct {
		Notes []map[string]any `json:"notes"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if len(applied.Notes) != 2 {
		t.Fatalf("notes: %+v", applied.Notes)
	}
	for _, note := range applied.Notes {
		if note["color"] != "#c47a20" || note["icon"] != "flag" {
			t.Fatalf("appearance: %+v", note)
		}
		id := int64(note["id"].(float64))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notes/"+strconv.FormatInt(id, 10)+"/icon", nil))
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), raw) {
			t.Fatalf("icon %d: %d %q", id, w.Code, w.Body.String())
		}
	}
	leftName := strconv.FormatInt(left, 10) + ".png"
	rightName := strconv.FormatInt(right, 10) + ".png"
	if _, err := os.Stat(filepath.Join(s.DataDir, "note-icons", leftName)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.DataDir, "note-icons", rightName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.DataDir, "note-icons", strconv.FormatInt(source, 10)+".png"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notes/"+strconv.FormatInt(left, 10)+"/icon", nil))
	if !bytes.Equal(w.Body.Bytes(), raw) {
		t.Fatalf("child icon followed the source: %q", w.Body.String())
	}

	miss := doJSON(t, h, http.MethodPost, "/api/notes/icons", map[string]any{"ids": []int64{left, 999999}, "color": "#111111"}, nil)
	if miss.Code != http.StatusNotFound {
		t.Fatalf("missing id %d %s", miss.Code, miss.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notes/"+strconv.FormatInt(left, 10), nil))
	var leftNote map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &leftNote); err != nil {
		t.Fatal(err)
	}
	if leftNote["color"] != "#c47a20" {
		t.Fatalf("partial write: %+v", leftNote)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("ids", strconv.FormatInt(left, 10)+","+strconv.FormatInt(right, 10))
	part, err := mw.CreateFormFile("file", "next.png")
	if err != nil {
		t.Fatal(err)
	}
	next := []byte("second-file")
	_, _ = part.Write(next)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/notes/icons", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("multipart %d %s", rec.Code, rec.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notes/"+strconv.FormatInt(right, 10)+"/icon", nil))
	if !bytes.Equal(w.Body.Bytes(), next) {
		t.Fatalf("shared upload: %q", w.Body.String())
	}
}
