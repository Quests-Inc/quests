package pdfdoc

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/go-pdf/fpdf"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const maxImageSide = 1600 // longest side kept in the PDF, in pixels

// prepareImage decodes an attachment image (PNG, JPEG, GIF, WebP), bounds its size and returns bytes in a
// form fpdf's own decoder handles. JPEG is passed through untouched when it is small enough.
func prepareImage(data []byte) (out []byte, imgType string, w, h int, err error) {
	if len(data) > MaxImageBytes {
		return nil, "", 0, 0, fmt.Errorf("image is larger than %d MB", MaxImageBytes>>20)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		if c, werr := webp.DecodeConfig(bytes.NewReader(data)); werr == nil {
			cfg, format, err = c, "webp", nil
		}
	}
	if err != nil {
		return nil, "", 0, 0, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, "", 0, 0, fmt.Errorf("image dimensions %dx%d are out of range", cfg.Width, cfg.Height)
	}
	if format == "jpeg" && cfg.Width <= maxImageSide && cfg.Height <= maxImageSide {
		// fpdf reads progressive/CMYK JPEG poorly: only plain baseline passes through; decode to be sure.
		if _, derr := jpeg.Decode(bytes.NewReader(data)); derr == nil && bytes.HasPrefix(data, []byte{0xff, 0xd8}) && !isProgressive(data) {
			return data, "JPG", cfg.Width, cfg.Height, nil
		}
	}
	var img image.Image
	switch format {
	case "webp":
		img, err = webp.Decode(bytes.NewReader(data))
	case "gif":
		img, err = gif.Decode(bytes.NewReader(data))
	default:
		img, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, "", 0, 0, err
	}
	b := img.Bounds()
	tw, th := b.Dx(), b.Dy()
	if tw > maxImageSide || th > maxImageSide {
		s := float64(maxImageSide) / float64(max(tw, th))
		tw, th = max(1, int(float64(tw)*s)), max(1, int(float64(th)*s))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	if tw == b.Dx() && th == b.Dy() {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	} else {
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, "", 0, 0, err
	}
	return buf.Bytes(), "PNG", tw, th, nil
}

// isProgressive reports an SOF2 marker (progressive JPEG).
func isProgressive(b []byte) bool {
	for i := 2; i+3 < len(b); {
		if b[i] != 0xff {
			return false
		}
		m := b[i+1]
		if m == 0xc2 {
			return true
		}
		if m == 0xda { // start of scan: no more headers
			return false
		}
		i += 2 + int(b[i+2])<<8 + int(b[i+3])
	}
	return false
}

// image paints an attachment image as a block (or a placeholder when it cannot).
func (r *Renderer) image(ref imageRef) {
	placeholder := func(string) {
		st := bodyStyle()
		st.color, st.italic = colMuted, true
		var toks []token
		r.textTokens(st, fmt.Sprintf("▢ вложение недоступно (attachment=%d)", ref.id), &toks)
		r.paragraph(toks, ref.align)
	}
	if r.loader == nil || r.images >= MaxImages {
		placeholder("disabled")
		return
	}
	r.images++
	raw, err := r.loader(ref.id)
	if err != nil {
		placeholder(err.Error())
		return
	}
	data, typ, pw, ph, err := prepareImage(raw)
	if err != nil {
		placeholder(err.Error())
		return
	}
	// 96 dpi, but never wider than the column or taller than most of a page
	wmm, hmm := float64(pw)*25.4/96, float64(ph)*25.4/96
	if wmm > r.w {
		hmm, wmm = hmm*r.w/wmm, r.w
	}
	if maxH := (pageH - marginTop - marginBottom) * 0.8; hmm > maxH {
		wmm, hmm = wmm*maxH/hmm, maxH
	}
	r.ensure(hmm + 2)
	if !r.ok() {
		return
	}
	r.imgSeq++
	name := fmt.Sprintf("img%d", r.imgSeq)
	r.pdf.RegisterImageOptionsReader(name, imageOptions(typ), bytes.NewReader(data))
	x := r.x0
	switch ref.align {
	case "center":
		x += (r.w - wmm) / 2
	case "right":
		x += r.w - wmm
	}
	y := r.pdf.GetY()
	r.pdf.ImageOptions(name, x, y, wmm, hmm, false, imageOptions(typ), 0, "")
	r.pdf.SetY(y + hmm + 1.5)
	r.pageTop = false
}

func imageOptions(typ string) fpdf.ImageOptions { return fpdf.ImageOptions{ImageType: typ} }
