package pdfdoc

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var md = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList))

// refRe is the journal's own reference syntax (frontend/src/lib/js/refs.js).
var refRe = regexp.MustCompile(`\b(note|quest|questline|step|attachment|template)=(\d+)\b`)

// attachmentDest is how markdown names an image: ![alt|center](attachment=12).
var attachmentDest = regexp.MustCompile(`^attachment=(\d+)$`)

var alignSuffix = regexp.MustCompile(`^(.*?)\s*\|\s*(left|center|right)\s*$`)

// markdown parses src and paints it in the current column.
func (r *Renderer) markdown(src string) {
	if strings.TrimSpace(src) == "" || !r.ok() {
		return
	}
	b := []byte(src)
	doc := md.Parser().Parse(text.NewReader(b))
	r.blocks(doc, b)
}

func (r *Renderer) blocks(parent ast.Node, src []byte) {
	for n := parent.FirstChild(); n != nil && r.ok(); n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Heading:
			r.heading(v, src)
		case *ast.Paragraph:
			r.paragraph(r.inlineTokens(v, src, bodyStyle()), "")
			r.gap(2.4)
		case *ast.TextBlock:
			r.paragraph(r.inlineTokens(v, src, bodyStyle()), "")
			r.gap(0.8)
		case *ast.List:
			r.list(v, src)
		case *ast.Blockquote:
			r.blockquote(v, src)
		case *ast.FencedCodeBlock:
			r.code(string(v.Language(src)), blockLines(v, src))
		case *ast.CodeBlock:
			r.code("", blockLines(v, src))
		case *ast.ThematicBreak:
			r.gap(1.5)
			r.ensure(2)
			y := r.pdf.GetY()
			r.pdf.SetDrawColor(colRule[0], colRule[1], colRule[2])
			r.pdf.SetLineWidth(0.25)
			r.pdf.Line(r.x0, y, r.x0+r.w, y)
			r.pdf.SetY(y + 1.5)
		case *ast.HTMLBlock:
			// Raw HTML is not rendered (the app sanitizes it away too): show it as text.
			r.code("", blockLines(v, src))
		case *east.Table:
			r.table(v, src)
		default:
			if n.HasChildren() {
				r.blocks(n, src)
			}
		}
	}
}

func blockLines(n ast.Node, src []byte) []string {
	var lines []string
	l := n.Lines()
	for i := 0; i < l.Len(); i++ {
		seg := l.At(i)
		lines = append(lines, strings.TrimRight(string(seg.Value(src)), "\r\n"))
	}
	return lines
}

var headingSizes = [...]float64{0, 17, 14.5, 12.5, 11, 10.5, 10}

func (r *Renderer) heading(h *ast.Heading, src []byte) {
	lvl := h.Level
	if lvl < 1 {
		lvl = 1
	}
	if lvl > 6 {
		lvl = 6
	}
	st := style{fam: famSans, bold: true, size: headingSizes[lvl], color: rgb{17, 17, 17}}
	toks := r.inlineTokens(h, src, st)
	r.gap(3.2)
	// keep a heading with the first lines of what follows it
	r.ensure(r.lineHeight(st.size) + 2*r.lineHeight(bodyStyle().size))
	r.paragraph(toks, "")
	r.gap(1.3)
}

func (r *Renderer) list(l *ast.List, src []byte) {
	n := l.Start
	if n == 0 {
		n = 1
	}
	depth := 0
	for p := l.Parent(); p != nil; p = p.Parent() {
		if _, ok := p.(*ast.List); ok {
			depth++
		}
	}
	const indent = 6.0
	for item := l.FirstChild(); item != nil && r.ok(); item = item.NextSibling() {
		marker := "•"
		if depth > 0 {
			marker = "–"
		}
		if l.IsOrdered() {
			marker = strconv.Itoa(n) + "."
			n++
		}
		if startsWithCheckbox(item) {
			marker = ""
		}
		body := bodyStyle()
		r.ensure(r.lineHeight(body.size))
		y := r.pdf.GetY()
		fontMM := body.size * ptMM
		lh := r.lineHeight(body.size)
		if marker != "" {
			mw := r.measure(body, marker)
			r.setStyle(body)
			r.pdf.Text(r.x0+indent-1.2-mw, y+(lh-fontMM)/2+fontMM*0.8, marker)
		}
		saveX, saveW := r.x0, r.w
		r.x0 += indent
		r.w -= indent
		r.blocks(item, src)
		r.x0, r.w = saveX, saveW
	}
	r.gap(1.2)
}

func (r *Renderer) blockquote(q *ast.Blockquote, src []byte) {
	const indent = 4.5
	startPage, startY := r.pdf.PageNo(), r.pdf.GetY()
	saveX, saveW := r.x0, r.w
	r.x0 += indent
	r.w -= indent
	r.blocks(q, src)
	r.x0, r.w = saveX, saveW
	endPage, endY := r.pdf.PageNo(), r.pdf.GetY()
	cur := r.pdf.PageNo()
	r.pdf.SetDrawColor(colRule[0]-25, colRule[1]-25, colRule[2]-25)
	r.pdf.SetLineWidth(0.8)
	for p := startPage; p <= endPage; p++ {
		y0, y1 := marginTop, pageH-marginBottom
		if p == startPage {
			y0 = startY
		}
		if p == endPage {
			y1 = endY
		}
		r.pdf.SetPage(p)
		r.pdf.Line(r.x0+0.6, y0, r.x0+0.6, y1-1.2)
	}
	r.pdf.SetPage(cur)
	r.pdf.SetY(endY)
}

// inlineTokens flattens the inline children of n into styled tokens.
func (r *Renderer) inlineTokens(n ast.Node, src []byte, st style) []token {
	var out []token
	r.inline(n, src, st, &out)
	return out
}

func (r *Renderer) inline(parent ast.Node, src []byte, st style, out *[]token) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Text:
			r.textWithRefs(st, string(v.Segment.Value(src)), out)
			if v.HardLineBreak() || v.SoftLineBreak() { // the app renders soft breaks as breaks (marked breaks:true)
				*out = append(*out, token{st: st, brk: true})
			}
		case *ast.String:
			r.textTokens(st, string(v.Value), out)
		case *ast.Emphasis:
			s := st
			if v.Level >= 2 {
				s.bold = true
			} else {
				s.italic = true
			}
			r.inline(v, src, s, out)
		case *east.Strikethrough:
			s := st
			s.strike = true
			r.inline(v, src, s, out)
		case *ast.CodeSpan:
			s := st
			s.fam, s.size, s.bg = famMono, st.size*0.88, &colInlineB
			r.textTokens(s, plainText(v, src), out)
		case *ast.Link:
			s := st
			s.color, s.underline = colLink, true
			dest := string(v.Destination)
			if strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://") || strings.HasPrefix(dest, "mailto:") {
				s.link = dest
			}
			r.inline(v, src, s, out)
		case *ast.AutoLink:
			s := st
			s.color, s.underline = colLink, true
			url := string(v.URL(src))
			if v.AutoLinkType == ast.AutoLinkURL {
				s.link = url
			}
			r.textTokens(s, string(v.Label(src)), out)
		case *ast.Image:
			ref := imageRefOf(v, src)
			if ref == nil {
				r.textTokens(st, "▢ "+plainText(v, src), out)
				continue
			}
			*out = append(*out, token{img: ref})
		case *ast.RawHTML:
			var b strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				b.Write(seg.Value(src))
			}
			r.textTokens(st, b.String(), out)
		case *east.TaskCheckBox:
			s := st
			s.fam, s.bold = famSans, false
			mark := "☐ "
			if v.IsChecked {
				mark = "☑ "
			}
			r.textTokens(s, mark, out)
		default:
			r.inline(n, src, st, out)
		}
	}
}

// textWithRefs writes text, replacing journal refs that have a label by that label.
func (r *Renderer) textWithRefs(st style, text string, out *[]token) {
	if len(r.labels) == 0 || !refRe.MatchString(text) {
		r.textTokens(st, text, out)
		return
	}
	last := 0
	for _, m := range refRe.FindAllStringSubmatchIndex(text, -1) {
		kind, id := text[m[2]:m[3]], text[m[4]:m[5]]
		label, ok := r.labels[kind+":"+id]
		if !ok {
			continue
		}
		r.textTokens(st, text[last:m[0]], out)
		s := st
		s.color = colLink
		r.textTokens(s, label, out)
		last = m[1]
	}
	r.textTokens(st, text[last:], out)
}

func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(p ast.Node) {
		for c := p.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				b.Write(t.Segment.Value(src))
				continue
			}
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func imageRefOf(img *ast.Image, src []byte) *imageRef {
	m := attachmentDest.FindStringSubmatch(string(img.Destination))
	if m == nil {
		return nil
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil
	}
	alt, align := plainText(img, src), ""
	if am := alignSuffix.FindStringSubmatch(alt); am != nil {
		alt, align = am[1], strings.ToLower(am[2])
	}
	return &imageRef{id: id, alt: alt, align: align}
}

// startsWithCheckbox reports a GFM task item ("- [ ] …"): its checkbox replaces the bullet.
func startsWithCheckbox(item ast.Node) bool {
	first := item.FirstChild()
	if first == nil || first.FirstChild() == nil {
		return false
	}
	_, ok := first.FirstChild().(*east.TaskCheckBox)
	return ok
}
