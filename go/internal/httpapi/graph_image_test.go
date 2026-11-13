package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestUnboundMapImageAndThumbnail(t *testing.T) {
	dav := newFakeDAV()
	defer dav.Close()
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	h := s.Handler()
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 600))
	for y := range 600 {
		for x := range 1200 {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x * y), 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	w := postFile(t, h, "/api/attachments", "clipboard.png", "", raw.Bytes())
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a knowledge.AttachmentRead
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	if a.OwnerType != "workspace" {
		t.Fatal(a.OwnerType)
	}
	path := fmt.Sprintf("/api/attachments/%d/image?preview=1&thumb=128", a.ID)
	thumb := doJSON(t, h, "GET", path, nil, nil)
	if thumb.Code != 200 {
		t.Fatal(thumb.Code, thumb.Body.String())
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(thumb.Body.Bytes()))
	if err != nil || config.Width != 128 || config.Height != 64 {
		t.Fatal(config, err)
	}
	if thumb.Body.Len() >= raw.Len() {
		t.Fatal("thumbnail not reduced")
	}
	cached := doJSON(t, h, "GET", path, nil, map[string]string{"If-None-Match": thumb.Header().Get("ETag")})
	if cached.Code != 304 {
		t.Fatal(cached.Code)
	}
	original := doJSON(t, h, "GET", fmt.Sprintf("/api/attachments/%d/image?preview=1", a.ID), nil, nil)
	if original.Header().Get("ETag") == thumb.Header().Get("ETag") {
		t.Fatal("variant shares original ETag")
	}
	graph := doJSON(t, h, "GET", fmt.Sprintf("/api/graph?attachment=%d", a.ID), nil, nil)
	if graph.Code != 200 {
		t.Fatal(graph.Code, graph.Body.String())
	}
	list := doJSON(t, h, "GET", fmt.Sprintf("/api/workspaces/%d/attachments", a.OwnerID), nil, nil)
	if list.Code != 200 {
		t.Fatal(list.Code, list.Body.String())
	}
}
