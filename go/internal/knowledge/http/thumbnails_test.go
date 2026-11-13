package knowledgehttp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func thumbnailFixture(b *testing.B) []byte {
	b.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 1600, 900))
	for y := range 900 {
		for x := range 1600 {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x * y), 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		b.Fatal(err)
	}
	return raw.Bytes()
}
func BenchmarkGraphThumbnail(b *testing.B) {
	raw := thumbnailFixture(b)
	preview, err := resizeThumbnail(raw, 256)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(len(raw)), "original_bytes")
	b.ReportMetric(float64(len(preview)), "thumbnail_bytes")
	b.Run("generate", func(b *testing.B) {
		for b.Loop() {
			if _, err := resizeThumbnail(raw, 256); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(len(raw)), "original_bytes")
		b.ReportMetric(float64(len(preview)), "thumbnail_bytes")
	})
	thumbnailStore("benchmark-fixture", preview)
	b.Run("cached", func(b *testing.B) {
		for b.Loop() {
			if thumbnailCached("benchmark-fixture") == nil {
				b.Fatal("missing")
			}
		}
	})
}
