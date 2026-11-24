package pdfdoc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var bookMarker = regexp.MustCompile(`<!--\s*(span-id|table-id|row-id):\s*([a-z][a-z0-9_-]{0,63})\s*-->`)

func noteKey(id int64) string { return "note:" + strconv.FormatInt(id, 10) }

func (r *Renderer) placeAnchor(key string, y float64) {
	if link := r.bookLinks[key]; link != 0 {
		r.pdf.SetLink(link, y, r.pdf.PageNo())
	}
}

func (r *Renderer) inlineMarker(raw string) string {
	if r.bookLinks == nil {
		return ""
	}
	m := bookMarker.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return ""
	}
	switch m[1] {
	case "span-id":
		return noteKey(r.noteID) + "#" + m[2]
	case "row-id":
		if r.tableID != "" {
			return noteKey(r.noteID) + "#" + r.tableID + "/" + m[2]
		}
	}
	return ""
}

func (r *Renderer) blockMarkers(raw string) {
	for _, m := range bookMarker.FindAllStringSubmatch(raw, -1) {
		if m[1] == "table-id" {
			r.tableID = m[2]
		}
		if key := r.inlineMarker(m[0]); key != "" {
			r.placeAnchor(key, r.pdf.GetY())
		}
	}
}

// Book reserves the contents pages first, then fills their page numbers after layout.
// Each source is parsed separately so its local markers and heading levels stay local.
func (r *Renderer) book(chapters []Chapter) {
	if len(chapters) == 0 {
		return
	}
	r.bookLinks = make(map[string]int)
	for i, c := range chapters {
		if (i == 0 && c.Depth != 0) || (i > 0 && (c.Depth == 0 || c.Depth > chapters[i-1].Depth+1)) {
			r.fail(fmt.Errorf("pdfdoc: invalid chapter hierarchy"))
			return
		}
		key := noteKey(c.NoteID)
		if c.NoteID <= 0 || c.Depth < 0 || r.bookLinks[key] != 0 {
			r.fail(fmt.Errorf("pdfdoc: invalid chapter identity or depth"))
			return
		}
		r.bookLinks[key] = r.pdf.AddLink()
		// Only actual HTML comments become targets; code examples never do.
		src := []byte(c.Markdown)
		doc := md.Parser().Parse(text.NewReader(src))
		table := ""
		ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			var raw string
			switch v := n.(type) {
			case *ast.HTMLBlock:
				raw = strings.Join(blockLines(v, src), "\n")
			case *ast.RawHTML:
				for i := 0; i < v.Segments.Len(); i++ {
					seg := v.Segments.At(i)
					raw += string(seg.Value(src))
				}
			}
			for _, m := range bookMarker.FindAllStringSubmatch(raw, -1) {
				target := ""
				switch m[1] {
				case "table-id":
					table = m[2]
				case "span-id":
					target = m[2]
				case "row-id":
					if table != "" {
						target = table + "/" + m[2]
					}
				}
				if target != "" {
					r.bookLinks[key+"#"+target] = r.pdf.AddLink()
				}
			}
			return ast.WalkContinue, nil
		})
	}
	r.sectionLabel("Оглавление")
	type entry struct {
		page int
		y    float64
		left float64
	}
	entries := make([]entry, len(chapters))
	for i, c := range chapters {
		if !r.ok() {
			return
		}
		indent := float64(min(c.Depth, 8)) * 4
		st := r.bodyStyle()
		st.fam = famSans
		st.size = r.fontSize(10)
		st.internal = r.bookLinks[noteKey(c.NoteID)]
		st.color = colLink
		var toks []token
		r.textTokens(st, c.Title, &toks)
		width := r.w - indent - 14
		lines := r.layout(toks, width)
		// Very long titles may span pages; the number sits beside the final line.
		entries[i] = entry{}
		for _, l := range lines {
			r.ensure(l.h)
			if !r.ok() {
				return
			}
			r.drawLine(l, r.x0+indent, r.pdf.GetY(), width, "")
			r.pdf.SetY(r.pdf.GetY() + l.h)
			r.pageTop = false
		}
		entries[i].page = r.pdf.PageNo()
		entries[i].left = r.leftMargin(r.pdf.PageNo())
		entries[i].y = r.pdf.GetY() - lines[len(lines)-1].h
		r.gap(1.5)
	}
	pages := make([]int, len(chapters))
	for i, c := range chapters {
		if !r.ok() {
			return
		}
		if i == 0 || (c.Depth <= 1 && r.options.ChapterNewPage) {
			r.newPage()
		} else {
			r.gap(5)
		}
		r.noteID = c.NoteID
		r.tableID = ""
		r.headingOffset = min(c.Depth+1, 5)
		titleStyle := style{fam: r.headingFamily(), bold: true, size: r.fontSize(headingSizes[min(c.Depth+1, 6)]), color: r.textColor()}
		var toks []token
		r.textTokens(titleStyle, c.Title, &toks)
		lines := r.layout(toks, r.w)
		r.ensure(lines[0].h + 2*r.lineHeight(r.bodyStyle().size))
		pages[i] = r.pdf.PageNo()
		r.placeAnchor(noteKey(c.NoteID), r.pdf.GetY())
		// PDF outlines cannot skip levels.
		level := 0
		if i > 0 {
			level = min(c.Depth, min(chapters[i-1].Depth+1, 32))
		}
		r.pdf.Bookmark(c.Title, level, r.pdf.GetY())
		r.paragraph(toks, "")
		r.gap(3)
		src := []byte(c.Markdown)
		parsed := md.Parser().Parse(text.NewReader(src))
		// Notes often repeat their card title as the first Markdown heading.
		if h, ok := parsed.FirstChild().(*ast.Heading); ok && strings.TrimSpace(plainText(h, src)) == strings.TrimSpace(c.Title) {
			parsed.RemoveChild(parsed, h)
		}
		r.blocks(parsed, src)
	}
	lastPage, lastY := r.pdf.PageNo(), r.pdf.GetY()
	for i, e := range entries {
		r.pdf.SetPage(e.page)
		st := style{fam: famSans, size: r.fontSize(9), color: colMuted, internal: r.bookLinks[noteKey(chapters[i].NoteID)]}
		var toks []token
		r.textTokens(st, strconv.Itoa(pages[i]), &toks)
		lines := r.layout(toks, 12)
		r.drawLine(lines[0], e.left+r.w-12, e.y, 12, "right")
	}
	r.pdf.SetPage(lastPage)
	r.pdf.SetY(lastY)
	r.headingOffset = 0
}
