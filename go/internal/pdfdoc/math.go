package pdfdoc

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

var kindMath = ast.NewNodeKind("Math")

type mathNode struct {
	ast.BaseInline
	source  string
	display bool
}

func (n *mathNode) Kind() ast.NodeKind         { return kindMath }
func (n *mathNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type mathParser struct{}

func (mathParser) Trigger() []byte { return []byte{'$'} }
func (mathParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	if len(line) < 3 || line[0] != '$' {
		return nil
	}
	delim := "$"
	display := bytes.HasPrefix(line, []byte("$$"))
	if display {
		delim = "$$"
	} else if line[1] == ' ' || line[1] == '\t' {
		return nil
	}
	for i := len(delim); i+len(delim) <= len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if !bytes.HasPrefix(line[i:], []byte(delim)) {
			continue
		}
		if !display && (line[i-1] == ' ' || line[i-1] == '\t' || i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9') {
			continue
		}
		reader.Advance(i + len(delim))
		return &mathNode{source: string(line[len(delim):i]), display: display}
	}
	return nil
}

// Boxes use millimetres, with glyph positions measured from their top edge.
type mathGlyph struct {
	text         string
	x, y, size   float64
	bold, italic bool
}
type mathRule struct{ x, y, w float64 }
type mathBox struct {
	w, h, base float64
	glyphs     []mathGlyph
	rules      []mathRule
}

var mathFonts = sync.OnceValues(func() ([]*sfnt.Font, error) {
	var fonts []*sfnt.Font
	for _, name := range []string{"DejaVuSerif.ttf", "DejaVuSerif-Italic.ttf", "DejaVuSerif-Bold.ttf", "DejaVuSerif-BoldItalic.ttf"} {
		data, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			return nil, err
		}
		f, err := sfnt.Parse(data)
		if err != nil {
			return nil, err
		}
		fonts = append(fonts, f)
	}
	return fonts, nil
})

func (b *mathBox) add(c *mathBox, x, y float64) {
	for _, g := range c.glyphs {
		g.x += x
		g.y += y
		b.glyphs = append(b.glyphs, g)
	}
	for _, r := range c.rules {
		r.x += x
		r.y += y
		b.rules = append(b.rules, r)
	}
}
func mathJoin(a, b *mathBox) *mathBox {
	base := math.Max(a.base, b.base)
	out := &mathBox{w: a.w + b.w, base: base, h: base + math.Max(a.h-a.base, b.h-b.base)}
	out.add(a, 0, base-a.base)
	out.add(b, a.w, base-b.base)
	return out
}

type mathReader struct {
	src        []rune
	pos, depth int
	fonts      []*sfnt.Font
}

func (p *mathReader) glyph(s string, size float64, bold, italic bool) (*mathBox, error) {
	index := 0
	if bold {
		index += 2
	}
	if italic {
		index++
	}
	f := p.fonts[index]
	var buf sfnt.Buffer
	w := 0.0
	for _, c := range s {
		g, err := f.GlyphIndex(&buf, c)
		if err != nil || g == 0 {
			return nil, fmt.Errorf("unsupported math glyph %q", c)
		}
		a, err := f.GlyphAdvance(&buf, g, fixed.Int26_6(size*64), font.HintingNone)
		if err != nil {
			return nil, err
		}
		w += float64(a) / 64 * ptMM
	}
	return &mathBox{w: w, h: size * ptMM * 1.2, base: size * ptMM * 0.9, glyphs: []mathGlyph{{text: s, y: size * ptMM * 0.9, size: size, bold: bold, italic: italic}}}, nil
}
func (p *mathReader) group(size float64, bold, italic bool) (*mathBox, error) {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
	if p.pos >= len(p.src) || p.src[p.pos] != '{' {
		return p.atom(size, bold, italic)
	}
	p.pos++
	p.depth++
	if p.depth > 24 {
		return nil, fmt.Errorf("math too deep")
	}
	b, err := p.sequence(size, bold, italic, true)
	p.depth--
	return b, err
}
func (p *mathReader) sequence(size float64, bold, italic, group bool) (*mathBox, error) {
	out := &mathBox{}
	for p.pos < len(p.src) {
		if p.src[p.pos] == '}' {
			if !group {
				return nil, fmt.Errorf("unbalanced math group")
			}
			p.pos++
			return out, nil
		}
		if unicode.IsSpace(p.src[p.pos]) {
			p.pos++
			continue
		}
		b, err := p.atom(size, bold, italic)
		if err != nil {
			return nil, err
		}
		var sub, sup *mathBox
		for p.pos < len(p.src) && (p.src[p.pos] == '_' || p.src[p.pos] == '^') {
			op := p.src[p.pos]
			p.pos++
			v, err := p.group(size*0.7, bold, italic)
			if err != nil {
				return nil, err
			}
			if op == '_' {
				if sub != nil {
					return nil, fmt.Errorf("duplicate subscript")
				}
				sub = v
			} else {
				if sup != nil {
					return nil, fmt.Errorf("duplicate superscript")
				}
				sup = v
			}
		}
		if sub != nil || sup != nil {
			top := 0.0
			if sup != nil {
				top = math.Max(0, sup.h-b.base*0.65)
			}
			c := &mathBox{w: b.w, h: b.h + top, base: b.base + top}
			c.add(b, 0, top)
			if sup != nil {
				c.add(sup, b.w, top+b.base*0.35-sup.h)
				c.w = math.Max(c.w, b.w+sup.w)
			}
			if sub != nil {
				y := top + b.base*0.9
				c.add(sub, b.w, y)
				c.w = math.Max(c.w, b.w+sub.w)
				c.h = math.Max(c.h, y+sub.h)
			}
			b = c
		}
		out = mathJoin(out, b)
	}
	if group {
		return nil, fmt.Errorf("unclosed math group")
	}
	return out, nil
}

var mathSymbols = map[string]string{"Phi": "Φ", "phi": "φ", "alpha": "α", "beta": "β", "gamma": "γ", "Gamma": "Γ", "delta": "δ", "Delta": "Δ", "epsilon": "ε", "theta": "θ", "Theta": "Θ", "lambda": "λ", "mu": "μ", "pi": "π", "rho": "ρ", "sigma": "σ", "Sigma": "Σ", "tau": "τ", "omega": "ω", "Omega": "Ω", "sum": "∑", "prod": "∏", "int": "∫", "infty": "∞", "partial": "∂", "nabla": "∇", "cdot": "·", "times": "×", "div": "÷", "pm": "±", "le": "≤", "leq": "≤", "ge": "≥", "geq": "≥", "ne": "≠", "neq": "≠", "approx": "≈", "equiv": "≡", "to": "→", "rightarrow": "→", "leftarrow": "←", "in": "∈", "notin": "∉", "subset": "⊂", "cup": "∪", "cap": "∩", "forall": "∀", "exists": "∃", "ldots": "…", "dots": "…", "sin": "sin", "cos": "cos", "tan": "tan", "log": "log", "ln": "ln", "exp": "exp", "min": "min", "max": "max", "lim": "lim"}

func (p *mathReader) atom(size float64, bold, italic bool) (*mathBox, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("missing math argument")
	}
	c := p.src[p.pos]
	p.pos++
	if c == '{' {
		p.pos--
		return p.group(size, bold, italic)
	}
	if c == '_' || c == '^' || c == '}' {
		return nil, fmt.Errorf("unexpected math token")
	}
	if c != '\\' {
		if strings.ContainsRune("=+-<>", c) {
			b, err := p.glyph(string(c), size, bold, false)
			if err != nil {
				return nil, err
			}
			pad := size * ptMM * 0.18
			out := &mathBox{w: b.w + 2*pad, h: b.h, base: b.base}
			out.add(b, pad, 0)
			return out, nil
		}
		return p.glyph(string(c), size, bold, italic && unicode.IsLetter(c))
	}
	start := p.pos
	for p.pos < len(p.src) && (p.src[p.pos] >= 'a' && p.src[p.pos] <= 'z' || p.src[p.pos] >= 'A' && p.src[p.pos] <= 'Z') {
		p.pos++
	}
	cmd := string(p.src[start:p.pos])
	if cmd == "" {
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("trailing escape")
		}
		c = p.src[p.pos]
		p.pos++
		switch c {
		case ',', ';', ' ', ':':
			return &mathBox{w: size * ptMM * 0.25}, nil
		case '!':
			return &mathBox{}, nil
		case '%', '{', '}', '_', '$', '#', '&':
			return p.glyph(string(c), size, bold, false)
		}
		return nil, fmt.Errorf("unsupported math escape")
	}
	switch cmd {
	case "frac", "dfrac", "tfrac":
		num, err := p.group(size*0.85, bold, italic)
		if err != nil {
			return nil, err
		}
		den, err := p.group(size*0.85, bold, italic)
		if err != nil {
			return nil, err
		}
		pad, gap := size*ptMM*0.12, size*ptMM*0.15
		w := math.Max(num.w, den.w) + 2*pad
		out := &mathBox{w: w, h: num.h + den.h + 2*gap, base: num.h + gap + size*ptMM*0.25}
		out.add(num, (w-num.w)/2, 0)
		out.add(den, (w-den.w)/2, num.h+2*gap)
		out.rules = append(out.rules, mathRule{y: num.h + gap, w: w})
		return out, nil
	case "sqrt":
		b, err := p.group(size, bold, italic)
		if err != nil {
			return nil, err
		}
		root, err := p.glyph("√", size*1.15, bold, false)
		if err != nil {
			return nil, err
		}
		out := mathJoin(root, b)
		out.rules = append(out.rules, mathRule{x: root.w, y: 0.15, w: b.w})
		return out, nil
	case "mathbf":
		return p.group(size, true, false)
	case "mathrm", "operatorname":
		return p.group(size, bold, false)
	case "mathit":
		return p.group(size, bold, true)
	case "text":
		if p.pos >= len(p.src) || p.src[p.pos] != '{' {
			return nil, fmt.Errorf("missing text group")
		}
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '}' {
			if p.src[p.pos] == '{' || p.src[p.pos] == '\\' {
				return nil, fmt.Errorf("unsupported text group")
			}
			p.pos++
		}
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unclosed text group")
		}
		v := string(p.src[start:p.pos])
		p.pos++
		return p.glyph(v, size, bold, false)
	case "left", "right":
		return p.atom(size, bold, false)
	case "quad", "qquad":
		w := size * ptMM
		if cmd == "qquad" {
			w *= 2
		}
		return &mathBox{w: w}, nil
	}
	if v, ok := mathSymbols[cmd]; ok {
		return p.glyph(v, size, bold, false)
	}
	return nil, fmt.Errorf("unsupported math command %s", cmd)
}
func parseMath(source string, size float64) (*mathBox, error) {
	if len(source) > 4096 || strings.Count(source, "{") > 100 {
		return nil, fmt.Errorf("math too large")
	}
	fonts, err := mathFonts()
	if err != nil {
		return nil, err
	}
	p := mathReader{src: []rune(source), fonts: fonts}
	return p.sequence(size, false, true, false)
}

func (r *Renderer) mathTokens(n *mathNode, st style, out *[]token) {
	r.mathCount++
	var box *mathBox
	var err error
	if r.mathCount <= 200 {
		box, err = parseMath(n.source, st.size)
	} else {
		err = fmt.Errorf("too many formulas")
	}
	if err != nil {
		delim := "$"
		if n.display {
			delim = "$$"
		}
		r.textTokens(st, delim+n.source+delim+" [формула не поддерживается в PDF]", out)
		return
	}
	scale := math.Min(1, r.w/box.w)
	if n.display {
		*out = append(*out, token{st: st, brk: true})
	}
	*out = append(*out, token{st: st, w: box.w * scale, math: box, mathScale: scale})
	if n.display {
		*out = append(*out, token{st: st, brk: true})
	}
}
func (r *Renderer) drawMath(b *mathBox, x, y, scale float64) {
	r.pdf.SetTextColor(colText[0], colText[1], colText[2])
	r.pdf.SetDrawColor(colText[0], colText[1], colText[2])
	r.pdf.SetLineWidth(0.15 * scale)
	for _, g := range b.glyphs {
		st := ""
		if g.bold {
			st += "B"
		}
		if g.italic {
			st += "I"
		}
		r.pdf.SetFont(famSerif, st, g.size*scale)
		r.pdf.Text(x+g.x*scale, y+g.y*scale, g.text)
	}
	for _, v := range b.rules {
		r.pdf.Line(x+v.x*scale, y+v.y*scale, x+(v.x+v.w)*scale, y+v.y*scale)
	}
}

var kindMathBlock = ast.NewNodeKind("MathBlock")

type mathBlock struct {
	ast.BaseBlock
	source string
	closed bool
}

func (n *mathBlock) Kind() ast.NodeKind         { return kindMathBlock }
func (n *mathBlock) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }
func (mathBlockParser) Open(_ ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	raw := strings.TrimSpace(string(line))
	if !strings.HasPrefix(raw, "$$") {
		return nil, parser.NoChildren
	}
	rest := raw[2:]
	n := &mathBlock{}
	if i := strings.Index(rest, "$$"); i >= 0 {
		if strings.TrimSpace(rest[i+2:]) != "" {
			return nil, parser.NoChildren
		}
		n.source = rest[:i]
		n.closed = true
	} else {
		n.source = rest
	}
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}
func (mathBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	n := node.(*mathBlock)
	if n.closed {
		return parser.Close
	}
	line, _ := reader.PeekLine()
	raw := string(line)
	if i := strings.Index(raw, "$$"); i >= 0 && strings.TrimSpace(raw[i+2:]) == "" {
		n.source += "\n" + raw[:i]
		n.closed = true
	} else {
		n.source += "\n" + raw
	}
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}
func (mathBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (mathBlockParser) CanInterruptParagraph() bool                 { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool                 { return false }
