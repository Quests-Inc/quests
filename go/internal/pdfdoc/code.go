package pdfdoc

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

type codeSeg struct {
	text   string
	color  rgb
	bold   bool
	italic bool
}

// highlight tokenises code for lang (empty or unknown: plain text) and returns it split into source lines.
func highlight(lang string, lines []string) [][]codeSeg {
	plain := func() [][]codeSeg {
		out := make([][]codeSeg, len(lines))
		for i, l := range lines {
			out[i] = []codeSeg{{text: l, color: rgb{40, 40, 40}}}
		}
		return out
	}
	if lang == "" {
		return plain()
	}
	lx := lexers.Get(lang)
	if lx == nil {
		return plain()
	}
	it, err := chroma.Coalesce(lx).Tokenise(nil, strings.Join(lines, "\n")+"\n")
	if err != nil {
		return plain()
	}
	style := styles.Get("github")
	out := [][]codeSeg{nil}
	for t := it(); t != chroma.EOF; t = it() {
		e := style.Get(t.Type)
		col := rgb{40, 40, 40}
		if e.Colour.IsSet() {
			col = rgb{int(e.Colour.Red()), int(e.Colour.Green()), int(e.Colour.Blue())}
		}
		parts := strings.Split(t.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				out = append(out, nil)
			}
			if p != "" {
				cur := len(out) - 1
				out[cur] = append(out[cur], codeSeg{text: p, color: col, bold: e.Bold == chroma.Yes, italic: e.Italic == chroma.Yes})
			}
		}
	}
	if n := len(out); n > 0 && len(out[n-1]) == 0 { // the trailing newline added above
		out = out[:n-1]
	}
	return out
}

// code paints a fenced or indented code block with a grey background, wrapping long lines by columns.
func (r *Renderer) code(lang string, lines []string) {
	const padX, size = 2.2, 8.6
	st := style{fam: famMono, size: size, color: rgb{40, 40, 40}}
	lh := r.lineHeight(size) * 0.92
	charW := r.measure(st, "0")
	cols := int((r.w - 2*padX) / charW)
	if cols < 10 {
		cols = 10
	}
	r.gap(0.6)
	fillRow := func(h float64) {
		r.pdf.SetFillColor(colCodeBG[0], colCodeBG[1], colCodeBG[2])
		r.pdf.Rect(r.x0, r.pdf.GetY(), r.w, h, "F")
	}
	if tag := strings.ToUpper(strings.TrimSpace(lang)); tag != "" {
		r.ensure(lh + 4)
		if !r.ok() {
			return
		}
		fillRow(4.2)
		ts := style{fam: famSans, size: 6.5, color: rgb{140, 140, 140}}
		r.setStyle(ts)
		r.pdf.Text(r.x0+padX, r.pdf.GetY()+3, tag)
		r.pdf.SetY(r.pdf.GetY() + 4.2)
		r.pageTop = false
	} else {
		r.ensure(lh + 2)
		if !r.ok() {
			return
		}
		fillRow(1.6)
		r.pdf.SetY(r.pdf.GetY() + 1.6)
	}
	for _, segs := range highlight(lang, lines) {
		// expand tabs and cut the line into rows of at most cols characters
		var rows [][]codeSeg
		var row []codeSeg
		n := 0
		for _, s := range segs {
			text := strings.ReplaceAll(s.text, "\t", "    ")
			for _, ch := range text {
				if n == cols {
					rows = append(rows, row)
					row, n = nil, 0
				}
				if k := len(row); k > 0 && row[k-1].color == s.color && row[k-1].bold == s.bold && row[k-1].italic == s.italic {
					row[k-1].text += string(ch)
				} else {
					row = append(row, codeSeg{text: string(ch), color: s.color, bold: s.bold, italic: s.italic})
				}
				n++
			}
		}
		rows = append(rows, row)
		for _, rw := range rows {
			r.ensure(lh)
			if !r.ok() {
				return
			}
			y := r.pdf.GetY()
			fillRow(lh)
			x := r.x0 + padX
			base := y + (lh-size*ptMM)/2 + size*ptMM*0.8
			for _, s := range rw {
				cs := st
				cs.color, cs.bold = s.color, s.bold
				r.setStyle(cs)
				r.pdf.Text(x, base, s.text)
				x += charW * float64(len([]rune(s.text)))
			}
			r.pdf.SetY(y + lh)
			r.pageTop = false
		}
	}
	fillRow(1.6)
	r.pdf.SetY(r.pdf.GetY() + 1.6)
	r.gap(2)
}
