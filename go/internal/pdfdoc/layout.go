package pdfdoc

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"
)

const (
	ptMM         = 25.4 / 72 // one point in millimetres
	pageW        = 210.0
	pageH        = 297.0
	marginX      = 16.0
	marginTop    = 16.0
	marginBottom = 18.0
	lineFactor   = 1.45
)

// ErrTooLong is returned when a document would exceed MaxPages.
var ErrTooLong = errors.New("pdfdoc: document is too long")

type rgb [3]int

var (
	colText    = rgb{26, 26, 26}
	colMuted   = rgb{110, 110, 110}
	colLink    = rgb{26, 95, 180}
	colRule    = rgb{205, 205, 205}
	colCodeBG  = rgb{244, 244, 244}
	colHeadBG  = rgb{240, 240, 240}
	colInlineB = rgb{234, 234, 234}
	colGreen   = rgb{38, 140, 70}
)

// style is the look of a run of text.
type style struct {
	fam       string
	bold      bool
	italic    bool
	size      float64 // pt
	color     rgb
	bg        *rgb
	link      string // external URL: becomes a link annotation
	strike    bool
	underline bool
}

func (s style) fpdfStyle() string {
	out := ""
	if s.bold {
		out += "B"
	}
	if s.italic && s.fam == famSerif { // the other families have no italic cut
		out += "I"
	}
	return out
}

func bodyStyle() style { return style{fam: famSerif, size: 10, color: colText} }

// token is a word, a space or a forced break; img marks an image (laid out as its own block).
type token struct {
	text  string
	st    style
	w     float64
	space bool
	brk   bool
	img   *imageRef
}

type imageRef struct {
	id    int64
	alt   string
	align string
}

type placedTok struct {
	tok token
	x   float64
}

type line struct {
	items []placedTok
	w, h  float64
	size  float64 // largest font size on the line (pt)
}

// Renderer lays text and blocks out on a fpdf page.
type Renderer struct {
	ctx    context.Context
	pdf    *fpdf.Fpdf
	loader ImageLoader
	labels map[string]string
	err    error

	x0, w   float64 // current left edge and width of the content column
	images  int
	imgSeq  int
	pageTop bool // nothing drawn on the current page yet
}

func (r *Renderer) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *Renderer) ok() bool {
	if r.err != nil {
		return false
	}
	if err := r.ctx.Err(); err != nil {
		r.fail(err)
		return false
	}
	if err := r.pdf.Error(); err != nil {
		r.fail(err)
		return false
	}
	return true
}

func (r *Renderer) newPage() {
	if r.pdf.PageNo() >= MaxPages {
		r.fail(ErrTooLong)
		return
	}
	r.pdf.AddPage()
	r.pdf.SetY(marginTop)
	r.pageTop = true
}

// ensure makes room for h mm below the cursor, starting a new page when it is not there.
func (r *Renderer) ensure(h float64) {
	if !r.ok() {
		return
	}
	if r.pdf.GetY()+h > pageH-marginBottom && !r.pageTop {
		r.newPage()
	}
}

// gap moves the cursor down unless the page is still empty.
func (r *Renderer) gap(h float64) {
	if r.pageTop || !r.ok() {
		return
	}
	r.pdf.SetY(r.pdf.GetY() + h)
}

func (r *Renderer) setStyle(st style) {
	r.pdf.SetFont(st.fam, st.fpdfStyle(), st.size)
	r.pdf.SetTextColor(st.color[0], st.color[1], st.color[2])
}

func (r *Renderer) measure(st style, text string) float64 {
	r.pdf.SetFont(st.fam, st.fpdfStyle(), st.size)
	return r.pdf.GetStringWidth(text)
}

func (r *Renderer) lineHeight(size float64) float64 { return size * ptMM * lineFactor }

// textTokens splits text into word and space tokens of style st.
func (r *Renderer) textTokens(st style, text string, out *[]token) {
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			t := cur.String()
			*out = append(*out, token{text: t, st: st, w: r.measure(st, t)})
			cur.Reset()
		}
	}
	space := false
	for _, ch := range text {
		switch ch {
		case ' ', '\t', ' ':
			if !space {
				flush()
				*out = append(*out, token{text: " ", st: st, w: r.measure(st, " "), space: true})
				space = true
			}
		case '\n', '\r':
			flush()
			space = false
			if ch == '\n' {
				*out = append(*out, token{st: st, brk: true})
			}
		default:
			space = false
			cur.WriteRune(ch)
		}
	}
	flush()
}

// layout wraps tokens into lines of at most width mm.
func (r *Renderer) layout(toks []token, width float64) []line {
	var lines []line
	var cur line
	x := 0.0
	defSize := bodyStyle().size
	finish := func() {
		// drop trailing spaces
		for len(cur.items) > 0 && cur.items[len(cur.items)-1].tok.space {
			last := cur.items[len(cur.items)-1]
			cur.items = cur.items[:len(cur.items)-1]
			x -= last.tok.w
		}
		if cur.size == 0 {
			cur.size = defSize
		}
		cur.w = x
		cur.h = r.lineHeight(cur.size)
		lines = append(lines, cur)
		cur = line{}
		x = 0
	}
	place := func(t token) {
		cur.items = append(cur.items, placedTok{tok: t, x: x})
		x += t.w
		if t.st.size > cur.size {
			cur.size = t.st.size
		}
	}
	for _, t := range toks {
		switch {
		case t.brk:
			if t.st.size > cur.size {
				cur.size = t.st.size
			}
			finish()
		case t.space:
			if len(cur.items) == 0 {
				continue
			}
			place(t)
		default:
			if x+t.w > width+0.01 && len(cur.items) > 0 {
				finish()
			}
			if t.w <= width+0.01 {
				place(t)
				continue
			}
			// a word wider than the line: cut it by characters
			rest := t.text
			for rest != "" {
				fit := 0
				acc := 0.0
				for i, ch := range rest {
					cw := r.measure(t.st, string(ch))
					if acc+cw > width-x && i > 0 {
						break
					}
					acc += cw
					fit = i + utf8.RuneLen(ch)
				}
				if fit == 0 {
					fit = utf8.RuneLen([]rune(rest)[0])
				}
				piece := rest[:fit]
				place(token{text: piece, st: t.st, w: r.measure(t.st, piece)})
				rest = rest[fit:]
				if rest != "" {
					finish()
				}
			}
		}
	}
	if len(cur.items) > 0 || len(lines) == 0 {
		finish()
	}
	return lines
}

// drawLine paints one laid-out line with its top-left corner at (x, y); align is "", "center" or "right"
// within width.
func (r *Renderer) drawLine(l line, x, y, width float64, align string) {
	off := 0.0
	switch align {
	case "center":
		off = (width - l.w) / 2
	case "right":
		off = width - l.w
	}
	if off < 0 {
		off = 0
	}
	fontMM := l.size * ptMM
	base := y + (l.h-fontMM)/2 + fontMM*0.8
	for _, it := range l.items {
		t := it.tok
		tx := x + off + it.x
		if t.st.bg != nil {
			r.pdf.SetFillColor(t.st.bg[0], t.st.bg[1], t.st.bg[2])
			r.pdf.Rect(tx, base-fontMM*0.85, t.w, fontMM*1.2, "F")
		}
		if t.text == "" {
			continue
		}
		r.setStyle(t.st)
		r.pdf.Text(tx, base, t.text)
		if t.st.underline || t.st.strike {
			r.pdf.SetDrawColor(t.st.color[0], t.st.color[1], t.st.color[2])
			r.pdf.SetLineWidth(0.15)
			if t.st.underline {
				r.pdf.Line(tx, base+0.5, tx+t.w, base+0.5)
			}
			if t.st.strike {
				r.pdf.Line(tx, base-fontMM*0.28, tx+t.w, base-fontMM*0.28)
			}
		}
		if t.st.link != "" {
			r.pdf.LinkString(tx, base-fontMM*0.85, t.w, fontMM*1.2, t.st.link)
		}
	}
}

// paragraph lays tokens out in the current column and paints them, breaking pages between lines.
// Images in the token stream end the text before them, are drawn as blocks, and the text goes on after.
func (r *Renderer) paragraph(toks []token, align string) {
	var seg []token
	flush := func() {
		if len(seg) == 0 {
			return
		}
		for _, l := range r.layout(seg, r.w) {
			r.ensure(l.h)
			if !r.ok() {
				return
			}
			r.drawLine(l, r.x0, r.pdf.GetY(), r.w, align)
			r.pdf.SetY(r.pdf.GetY() + l.h)
			r.pageTop = false
		}
		seg = nil
	}
	for _, t := range toks {
		if t.img != nil {
			flush()
			r.image(*t.img)
			continue
		}
		seg = append(seg, t)
	}
	flush()
}

func hexColor(s string, def rgb) rgb {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return def
	}
	var c rgb
	for i := 0; i < 3; i++ {
		v := 0
		for _, ch := range s[i*2 : i*2+2] {
			v <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				v |= int(ch - '0')
			case ch >= 'a' && ch <= 'f':
				v |= int(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				v |= int(ch-'A') + 10
			default:
				return def
			}
		}
		c[i] = v
	}
	return c
}
