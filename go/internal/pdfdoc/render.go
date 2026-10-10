package pdfdoc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Render writes doc as a PDF to w. loader may be nil (images then show a placeholder).
func Render(ctx context.Context, doc Document, loader ImageLoader, w io.Writer) error {
	total := len(doc.Title)
	for _, b := range doc.Blocks {
		total += len(b.Markdown)
		for _, s := range b.Steps {
			total += len(s.Markdown) + len(s.Title)
		}
	}
	if total > MaxMarkdownBytes {
		return fmt.Errorf("pdfdoc: document is larger than %d MB", MaxMarkdownBytes>>20)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginX, marginTop, marginX)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(true)
	title := strings.TrimSpace(doc.Title)
	pdf.SetTitle(title, true)
	pdf.SetCreator("Quests", true)
	if err := registerFonts(pdf); err != nil {
		return err
	}
	pdf.AliasNbPages("{nb}")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-11)
		pdf.SetFont(famSans, "", 7.5)
		pdf.SetTextColor(colMuted[0], colMuted[1], colMuted[2])
		pdf.CellFormat(0, 4, fmt.Sprintf("%d / {nb}", pdf.PageNo()), "", 0, "C", false, 0, "")
	})

	r := &Renderer{ctx: ctx, pdf: pdf, loader: loader, labels: doc.Labels, x0: marginX, w: pageW - 2*marginX}
	r.newPage()
	r.header(doc)
	for _, b := range doc.Blocks {
		if !r.ok() {
			break
		}
		r.block(b)
	}
	if r.err != nil {
		return r.err
	}
	return pdf.Output(w)
}

func (r *Renderer) header(doc Document) {
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		title = "Без названия"
	}
	ts := style{fam: famSans, bold: true, size: 19, color: rgb{17, 17, 17}}
	var toks []token
	r.textTokens(ts, title, &toks)
	r.paragraph(toks, "")
	r.gap(1)

	if len(doc.Meta) > 0 {
		var meta []token
		for i, c := range doc.Meta {
			if i > 0 {
				r.textTokens(style{fam: famSans, size: 8.8, color: colMuted}, "  ·  ", &meta)
			}
			r.textTokens(style{fam: famSans, size: 8.8, color: hexColor(c.Color, colMuted)}, c.Text, &meta)
		}
		r.paragraph(meta, "")
	}
	if doc.Footnote != "" {
		var foot []token
		r.textTokens(style{fam: famSans, size: 8, color: colMuted}, doc.Footnote, &foot)
		r.paragraph(foot, "")
	}
	r.gap(4)
}

func (r *Renderer) sectionLabel(label string) {
	if label == "" {
		return
	}
	r.gap(3)
	r.ensure(12)
	var toks []token
	r.textTokens(style{fam: famSans, bold: true, size: 7.8, color: colMuted}, strings.ToUpper(label), &toks)
	r.paragraph(toks, "")
	y := r.pdf.GetY() + 0.4
	r.pdf.SetDrawColor(colRule[0], colRule[1], colRule[2])
	r.pdf.SetLineWidth(0.25)
	r.pdf.Line(r.x0, y, r.x0+r.w, y)
	r.pdf.SetY(y + 2)
}

func (r *Renderer) muted(text string) {
	var toks []token
	r.textTokens(style{fam: famSerif, italic: true, size: 9.5, color: colMuted}, text, &toks)
	r.paragraph(toks, "")
}

func (r *Renderer) block(b Block) {
	switch b.Type {
	case BlockMarkdown:
		if strings.TrimSpace(b.Markdown) == "" {
			if b.Empty == "" {
				return
			}
			r.sectionLabel(b.Label)
			r.muted(b.Empty)
			return
		}
		r.sectionLabel(b.Label)
		r.markdown(b.Markdown)
	case BlockSteps:
		r.sectionLabel(b.Label)
		if len(b.Steps) == 0 {
			r.muted(orDefault(b.Empty, "Шагов нет"))
			return
		}
		for _, s := range b.Steps {
			r.step(s)
		}
	case BlockFacts:
		if len(b.Facts) == 0 {
			return
		}
		r.gap(3)
		r.sectionLabel(b.Label)
		for _, f := range b.Facts {
			r.fact(f)
		}
	case BlockList:
		if len(b.Items) == 0 {
			return
		}
		r.sectionLabel(b.Label)
		for _, it := range b.Items {
			var toks []token
			st := bodyStyle()
			st.size = 9.8
			r.textTokens(st, "•  "+it, &toks)
			r.paragraph(toks, "")
		}
		r.gap(2)
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (r *Renderer) step(s Step) {
	const markW, progW = 6.5, 14.0
	mark, markCol := "○", colMuted
	if s.Done {
		mark, markCol = "✓", colGreen
	}
	title := bodyStyle()
	title.size = 10.2
	if s.Done {
		title.color = colMuted
	}
	var toks []token
	r.textTokens(title, s.Title, &toks)
	lines := r.layout(toks, r.w-markW-progW-2)
	if len(lines) == 0 {
		return
	}
	r.ensure(lines[0].h)
	if !r.ok() {
		return
	}
	y := r.pdf.GetY()
	ms := style{fam: famSans, size: 10.5, color: markCol}
	r.setStyle(ms)
	fontMM := 10.5 * ptMM
	r.pdf.Text(r.x0, y+(lines[0].h-fontMM)/2+fontMM*0.8, mark)
	if s.Progress != "" {
		ps := style{fam: famSans, size: 8.5, color: colMuted}
		pw := r.measure(ps, s.Progress)
		r.setStyle(ps)
		r.pdf.Text(r.x0+r.w-pw, y+(lines[0].h-8.5*ptMM)/2+8.5*ptMM*0.8, s.Progress)
	}
	for i, l := range lines {
		if i > 0 {
			r.ensure(l.h)
		}
		r.drawLine(l, r.x0+markW, r.pdf.GetY(), r.w-markW-progW-2, "")
		r.pdf.SetY(r.pdf.GetY() + l.h)
		r.pageTop = false
	}
	if strings.TrimSpace(s.Markdown) != "" {
		saveX, saveW := r.x0, r.w
		r.x0 += markW
		r.w -= markW
		r.gap(0.8)
		r.markdown(s.Markdown)
		r.x0, r.w = saveX, saveW
	}
	r.gap(1.6)
}

func (r *Renderer) fact(f Fact) {
	const labelW = 44.0
	ls := style{fam: famSans, size: 8.6, color: colMuted}
	vs := bodyStyle()
	vs.size = 9.8
	var lt, vt []token
	r.textTokens(ls, f.Label, &lt)
	r.textTokens(vs, f.Value, &vt)
	left := r.layout(lt, labelW-2)
	right := r.layout(vt, r.w-labelW)
	rows := max(len(left), len(right))
	for i := 0; i < rows; i++ {
		h := 0.0
		if i < len(left) {
			h = left[i].h
		}
		if i < len(right) && right[i].h > h {
			h = right[i].h
		}
		r.ensure(h)
		if !r.ok() {
			return
		}
		y := r.pdf.GetY()
		if i < len(left) {
			r.drawLine(left[i], r.x0, y, labelW-2, "")
		}
		if i < len(right) {
			r.drawLine(right[i], r.x0+labelW, y, r.w-labelW, "")
		}
		r.pdf.SetY(y + h)
		r.pageTop = false
	}
	r.gap(0.6)
}
