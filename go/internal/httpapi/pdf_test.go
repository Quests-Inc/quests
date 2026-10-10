package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	stdpng "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postPDF(t *testing.T, s *Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/export/pdf", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

func TestExportPDFRendersADocument(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	w := postPDF(t, s, map[string]any{
		"filename": "заметка.pdf",
		"document": map[string]any{
			"title":  "Заметка",
			"blocks": []map[string]any{{"type": "markdown", "markdown": "# Привет\n\nтекст с **акцентом**\n\n![x](attachment=999)"}},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("content type %q", ct)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("Content-Disposition %q does not carry the UTF-8 name", cd)
	}
}

func TestExportPDFRejectsBadInput(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	if w := postPDF(t, s, map[string]any{"document": map[string]any{}}); w.Code != http.StatusBadRequest {
		t.Fatalf("empty document: %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/export/pdf", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("garbage: %d", w.Code)
	}
}

// A real uploaded image reaches the PDF; one the caller cannot see (or that is not an image) becomes a placeholder.
func TestExportPDFEmbedsAttachmentImages(t *testing.T) {
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	clam := fakeClamd(t, "stream: OK")
	s := newAttachmentServer(t, dav.URL, clam, 0)
	h := s.Handler()
	qid := seedQuest(t, s)

	var png bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	if err := stdpng.Encode(&png, img); err != nil {
		t.Fatal(err)
	}
	up := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "pic.png", "", png.Bytes())
	if up.Code != 201 {
		t.Fatalf("upload png: %d %s", up.Code, up.Body.String())
	}
	var pic map[string]any
	_ = json.Unmarshal(up.Body.Bytes(), &pic)
	txt := postFile(t, h, fmt.Sprintf("/api/quests/%d/attachments", qid), "a.txt", "", []byte("plain"))
	var note map[string]any
	_ = json.Unmarshal(txt.Body.Bytes(), &note)

	export := func(md string) []byte {
		w := postPDF(t, s, map[string]any{"document": map[string]any{
			"title": "с картинкой", "blocks": []map[string]any{{"type": "markdown", "markdown": md}},
		}})
		if w.Code != http.StatusOK {
			t.Fatalf("export: %d %s", w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	images := func(pdf []byte) int { return bytes.Count(pdf, []byte("/Subtype /Image")) }

	// not an image, hidden from us, or missing: no image object, just a placeholder
	if n := images(export(fmt.Sprintf("![t](attachment=%d)\n\n![m](attachment=424242)", int64(note["id"].(float64))))); n != 0 {
		t.Fatalf("a non-image attachment must not be embedded, got %d image objects", n)
	}
	if n := images(export(fmt.Sprintf("![p](attachment=%d)", int64(pic["id"].(float64))))); n == 0 {
		t.Fatal("the uploaded png was not embedded")
	}
}
