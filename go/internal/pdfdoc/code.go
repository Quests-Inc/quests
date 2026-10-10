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
		rows := wrapCode(segs, cols)
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

type codeCell struct {
	ch     rune
	color  rgb
	bold   bool
	italic bool
}

// wrapCode expands tabs and cuts one source line into rows of at most cols characters. It breaks after a
// space or a punctuation mark when there is one in the second half of the row (not in the middle of a word)
// and indents continuation rows under the line's own indentation, so wrapped code stays readable.
func wrapCode(segs []codeSeg, cols int) [][]codeSeg {
	var cells []codeCell
	for _, s := range segs {
		for _, ch := range strings.ReplaceAll(s.text, "\t", "    ") {
			cells = append(cells, codeCell{ch, s.color, s.bold, s.italic})
		}
	}
	indent := 0
	for indent < len(cells) && cells[indent].ch == ' ' {
		indent++
	}
	cont := min(indent+2, cols/3)
	group := func(prefix int, cs []codeCell) []codeSeg {
		var out []codeSeg
		if prefix > 0 {
			out = append(out, codeSeg{text: strings.Repeat(" ", prefix), color: rgb{40, 40, 40}})
		}
		for _, c := range cs {
			if k := len(out); k > 0 && out[k-1].color == c.color && out[k-1].bold == c.bold && out[k-1].italic == c.italic {
				out[k-1].text += string(c.ch)
			} else {
				out = append(out, codeSeg{text: string(c.ch), color: c.color, bold: c.bold, italic: c.italic})
			}
		}
		return out
	}
	if len(cells) <= cols {
		return [][]codeSeg{group(0, cells)}
	}
	var rows [][]codeSeg
	pos, prefix := 0, 0
	for pos < len(cells) {
		capacity := cols - prefix
		end := min(pos+capacity, len(cells))
		if end < len(cells) {
			for i := end; i > pos+capacity/2; i-- { // last good break point in the row's second half
				if c := cells[i-1].ch; c == ' ' || c == ',' || c == ';' || c == '(' || c == ')' || c == '.' {
					end = i
					break
				}
			}
		}
		rows = append(rows, group(prefix, cells[pos:end]))
		pos, prefix = end, cont
		for pos < len(cells) && cells[pos].ch == ' ' && pos > 0 { // do not start a continuation with the break's own spaces
			pos++
		}
	}
	return rows
}
