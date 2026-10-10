package pdfdoc

import (
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
)

type cell struct {
	toks  []token
	align string
	min   float64
	max   float64
}

const (
	cellPadX = 1.8
	cellPadY = 1.1
)

func (r *Renderer) table(t *east.Table, src []byte) {
	var rows [][]cell
	headerRows := 0
	for n := t.FirstChild(); n != nil; n = n.NextSibling() {
		var cells []cell
		bold := false
		switch n.(type) {
		case *east.TableHeader:
			bold = true
			headerRows++
		case *east.TableRow:
		default:
			continue
		}
		col := 0
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			st := bodyStyle()
			st.size = 9.3
			st.bold = bold
			ce := cell{toks: r.inlineTokens(c, src, st)}
			if tc, ok := c.(*east.TableCell); ok {
				switch tc.Alignment {
				case east.AlignCenter:
					ce.align = "center"
				case east.AlignRight:
					ce.align = "right"
				}
			}
			for _, tk := range ce.toks {
				if tk.img != nil {
					continue
				}
				ce.max += tk.w
				if !tk.space && tk.w > ce.min {
					ce.min = tk.w
				}
			}
			cells = append(cells, ce)
			col++
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, rw := range rows {
		if len(rw) > cols {
			cols = len(rw)
		}
	}
	minW, maxW := make([]float64, cols), make([]float64, cols)
	for _, rw := range rows {
		for i, c := range rw {
			if c.min > minW[i] {
				minW[i] = c.min
			}
			if c.max > maxW[i] {
				maxW[i] = c.max
			}
		}
	}
	pad := 2 * cellPadX
	sumMin, sumMax := 0.0, 0.0
	for i := range minW {
		minW[i] += pad
		maxW[i] += pad
		sumMin += minW[i]
		sumMax += maxW[i]
	}
	widths := make([]float64, cols)
	switch {
	case sumMax <= r.w: // fits: spread the spare width in proportion
		for i := range widths {
			widths[i] = maxW[i] * r.w / sumMax
		}
	case sumMin >= r.w: // not even the longest words fit: shrink evenly
		for i := range widths {
			widths[i] = minW[i] * r.w / sumMin
		}
	default:
		extra := r.w - sumMin
		for i := range widths {
			widths[i] = minW[i] + extra*(maxW[i]-minW[i])/(sumMax-sumMin)
		}
	}

	type laid struct {
		lines []line
		h     float64
	}
	layRow := func(rw []cell) ([]laid, float64) {
		out := make([]laid, cols)
		h := 0.0
		for i, c := range rw {
			var plain []token
			for _, tk := range c.toks {
				if tk.img == nil {
					plain = append(plain, tk)
				}
			}
			ls := r.layout(plain, widths[i]-pad)
			ch := 2 * cellPadY
			for _, l := range ls {
				ch += l.h
			}
			out[i] = laid{lines: ls, h: ch}
			if ch > h {
				h = ch
			}
		}
		return out, h
	}
	drawRow := func(rw []cell, header bool) {
		lays, h := layRow(rw)
		r.ensure(h)
		if !r.ok() {
			return
		}
		y := r.pdf.GetY()
		x := r.x0
		for i := 0; i < cols; i++ {
			if header {
				r.pdf.SetFillColor(colHeadBG[0], colHeadBG[1], colHeadBG[2])
				r.pdf.Rect(x, y, widths[i], h, "F")
			}
			r.pdf.SetDrawColor(colRule[0], colRule[1], colRule[2])
			r.pdf.SetLineWidth(0.2)
			r.pdf.Rect(x, y, widths[i], h, "D")
			if i < len(rw) {
				ly := y + cellPadY
				for _, l := range lays[i].lines {
					r.drawLine(l, x+cellPadX, ly, widths[i]-pad, rw[i].align)
					ly += l.h
				}
			}
			x += widths[i]
		}
		r.pdf.SetY(y + h)
		r.pageTop = false
	}

	r.gap(0.8)
	for i, rw := range rows {
		startPage := r.pdf.PageNo()
		_, h := layRow(rw)
		r.ensure(h)
		if r.pdf.PageNo() != startPage && i >= headerRows { // continued on a new page: repeat the header
			for _, hr := range rows[:headerRows] {
				drawRow(hr, true)
			}
		}
		drawRow(rw, i < headerRows)
	}
	r.gap(3)
}

var _ ast.Node = (*east.Table)(nil)
