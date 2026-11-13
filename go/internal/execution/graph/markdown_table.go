package graph

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	tableast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type tableParams struct {
	Table  string   `json:"table"`
	Column int      `json:"column,omitempty"` // 1-based; zero returns full rows
	Omit   []string `json:"omit,omitempty"`   // explicitly excluded cell texts
}

var tableKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var tableMark = regexp.MustCompile(`^\s*table-id:\s*([a-z][a-z0-9_-]{0,63})\s*$`)
var rowMark = regexp.MustCompile(`^\s*row-id:\s*([a-z][a-z0-9_-]{0,63})\s*$`)

func tablePorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p tableParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if !tableKey.MatchString(p.Table) || p.Column < 0 || p.Column > 100 || len(p.Omit) > 100 {
		return nil, nil, fmt.Errorf("нужен table-id; column 0..100; исключений до 100")
	}
	return []Port{req("text", TStr), opt("column", TInt)}, []Port{req("value", TDoc), req("rows", TDoc), req("items", TDoc)}, nil
}

type markdownRow struct {
	ID    string   `json:"id"`
	Cells []string `json:"cells"`
}
type markdownTable struct {
	ID      string        `json:"id"`
	Headers []string      `json:"headers"`
	Rows    []markdownRow `json:"rows"`
}

func commentKey(raw string, pattern *regexp.Regexp) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "<!--") || !strings.HasSuffix(raw, "-->") {
		return ""
	}
	if m := pattern.FindStringSubmatch(raw[4 : len(raw)-3]); m != nil {
		return m[1]
	}
	return ""
}
func htmlBlockText(block *gast.HTMLBlock, src []byte) string {
	raw := append([]byte(nil), block.Lines().Value(src)...)
	if block.HasClosure() {
		raw = append(raw, block.ClosureLine.Value(src)...)
	}
	return string(raw)
}

func plainCell(n gast.Node, src []byte) string {
	var b strings.Builder
	_ = gast.Walk(n, func(n gast.Node, enter bool) (gast.WalkStatus, error) {
		if !enter {
			return gast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *gast.Text:
			b.Write(v.Segment.Value(src))
			if v.HardLineBreak() || v.SoftLineBreak() {
				b.WriteByte('\n')
			}
		case *gast.String:
			b.Write(v.Value)
		case *gast.RawHTML:
			if strings.EqualFold(strings.TrimSpace(string(v.Segments.Value(src))), "<br>") || strings.EqualFold(strings.TrimSpace(string(v.Segments.Value(src))), "<br/>") {
				b.WriteByte('\n')
			}
		}
		return gast.WalkContinue, nil
	})
	return strings.TrimSpace(html.UnescapeString(string(util.UnescapePunctuations([]byte(b.String())))))
}
func readMarkdownTable(source, id string) (markdownTable, error) {
	out := markdownTable{ID: id, Rows: []markdownRow{}}
	if len(source) > MaxTextBytes {
		return out, fmt.Errorf("текст таблицы больше 64 КБ")
	}
	src := []byte(source)
	doc := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser().Parse(text.NewReader(src))
	var found []gast.Node
	_ = gast.Walk(doc, func(n gast.Node, enter bool) (gast.WalkStatus, error) {
		if !enter || n.Kind() != tableast.KindTable {
			return gast.WalkContinue, nil
		}
		key := ""
		for prev := n.PreviousSibling(); prev != nil; prev = prev.PreviousSibling() {
			block, ok := prev.(*gast.HTMLBlock)
			if !ok {
				break
			}
			if k := commentKey(htmlBlockText(block, src), tableMark); k != "" {
				key = k
			} else {
				break
			}
		}
		if key == id {
			found = append(found, n)
		}
		return gast.WalkSkipChildren, nil
	})
	if len(found) != 1 {
		return out, fmt.Errorf("таблица %q: ожидалась одна метка, найдено %d", id, len(found))
	}
	ids := map[string]bool{}
	for row := found[0].FirstChild(); row != nil; row = row.NextSibling() {
		cells := []string{}
		key := ""
		marks := 0
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, plainCell(cell, src))
			_ = gast.Walk(cell, func(n gast.Node, enter bool) (gast.WalkStatus, error) {
				if v, ok := n.(*gast.RawHTML); ok && enter {
					if k := commentKey(string(v.Segments.Value(src)), rowMark); k != "" {
						key = k
						marks++
					}
				}
				return gast.WalkContinue, nil
			})
		}
		if row.Kind() == tableast.KindTableHeader {
			out.Headers = cells
			continue
		}
		if marks != 1 || ids[key] {
			return out, fmt.Errorf("строка %d: нужна одна уникальная row-id", len(out.Rows)+1)
		}
		if len(out.Rows) >= 1000 {
			return out, fmt.Errorf("строк больше 1000")
		}
		ids[key] = true
		out.Rows = append(out.Rows, markdownRow{key, cells})
	}
	if len(out.Rows) == 0 {
		return out, fmt.Errorf("таблица %q пуста", id)
	}
	return out, nil
}
func runMarkdownTable(rs *resolved, in Token, columns ...Token) (map[string]Token, error) {
	var p tableParams
	_ = strictParams(rs.node.Params, &p)
	if len(columns) > 0 && !columns[0].Exit {
		col, ok := columns[0].Value.(int64)
		if !ok || col < 0 || col > 100 {
			return nil, fmt.Errorf("column: целое число 0..100")
		}
		p.Column = int(col)
	}
	if p.Column == 0 && len(p.Omit) > 0 {
		return nil, fmt.Errorf("omit требует выбранную колонку")
	}
	t, err := readMarkdownTable(in.Value.(string), p.Table)
	if err != nil {
		return nil, err
	}
	if p.Column > len(t.Headers) {
		return nil, fmt.Errorf("колонка %d отсутствует", p.Column)
	}
	var items any = t.Rows
	if p.Column > 0 {
		list := []map[string]any{}
		for _, row := range t.Rows {
			if p.Column > len(row.Cells) {
				return nil, fmt.Errorf("в строке %s нет колонки %d", row.ID, p.Column)
			}
			value := row.Cells[p.Column-1]
			skip := false
			for _, omit := range p.Omit {
				skip = skip || value == omit
			}
			if !skip {
				list = append(list, map[string]any{"id": row.ID, "value": value})
			}
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("нет вариантов после исключений")
		}
		items = list
	}
	token := func(v any) Token { raw, _ := json.Marshal(v); return Token{Type: TDoc, Value: json.RawMessage(raw)} }
	out := map[string]Token{"value": token(t), "rows": token(t.Rows), "items": token(items)}
	for _, v := range out {
		if len(v.Value.(json.RawMessage)) > MaxTextBytes {
			return nil, fmt.Errorf("результат таблицы больше 64 КБ")
		}
	}
	return out, nil
}
