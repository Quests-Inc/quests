package knowledgehttp

import (
	"bytes"
	"container/list"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"sync"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type thumbnailEntry struct {
	key  string
	data []byte
}

var thumbs = struct {
	sync.Mutex
	items map[string]*list.Element
	order *list.List
	bytes int
}{items: map[string]*list.Element{}, order: list.New()}
var thumbSlots = make(chan struct{}, 2)

const maxThumbnailCache = 24 << 20

func thumbnailCached(key string) []byte {
	thumbs.Lock()
	defer thumbs.Unlock()
	if e := thumbs.items[key]; e != nil {
		thumbs.order.MoveToFront(e)
		return e.Value.(thumbnailEntry).data
	}
	return nil
}
func thumbnailStore(key string, data []byte) {
	thumbs.Lock()
	defer thumbs.Unlock()
	if _, ok := thumbs.items[key]; ok || len(data) > maxThumbnailCache {
		return
	}
	e := thumbs.order.PushFront(thumbnailEntry{key, data})
	thumbs.items[key] = e
	thumbs.bytes += len(data)
	for thumbs.bytes > maxThumbnailCache || len(thumbs.items) > 512 {
		old := thumbs.order.Back()
		v := old.Value.(thumbnailEntry)
		delete(thumbs.items, v.key)
		thumbs.bytes -= len(v.data)
		thumbs.order.Remove(old)
	}
}
func resizeThumbnail(raw []byte, limit int) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 20_000_000 {
		return nil, fmt.Errorf("image exceeds thumbnail pixel limit")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	w, h := config.Width, config.Height
	if w > limit || h > limit {
		if w >= h {
			h = max(1, h*limit/w)
			w = limit
		} else {
			w = max(1, w*limit/h)
			h = limit
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if dst.Opaque() {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 75})
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(&out, dst)
	}
	return out.Bytes(), err
}
func (h *Handler) serveThumbnail(w http.ResponseWriter, r *http.Request, path, key string, limit int) {
	key = path + ":" + key
	if data := thumbnailCached(key); data != nil {
		writeThumbnail(w, data)
		return
	}
	select {
	case thumbSlots <- struct{}{}:
		defer func() { <-thumbSlots }()
	case <-r.Context().Done():
		return
	}
	if data := thumbnailCached(key); data != nil {
		writeThumbnail(w, data)
		return
	}
	data, err := h.makeThumbnail(r.Context(), path, limit)
	if err != nil {
		writeErr(w, 422, "thumbnail unavailable: "+err.Error())
		return
	}
	thumbnailStore(key, data)
	writeThumbnail(w, data)
}
func (h *Handler) makeThumbnail(ctx context.Context, path string, limit int) ([]byte, error) {
	body, err := h.WebDAV.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, 25<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 25<<20 {
		return nil, fmt.Errorf("image exceeds thumbnail input limit")
	}
	return resizeThumbnail(raw, limit)
}
func writeThumbnail(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
