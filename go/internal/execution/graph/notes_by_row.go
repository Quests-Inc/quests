package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/refs"
)

const (
	maxHeads      = 64
	maxHeadDepth  = 32
	maxRowMatches = 1000
	maxRowErrors  = 30
	// HeadAll is the depth of `note=12+!`: every generation under the head, capped at maxHeadDepth.
	HeadAll = -1
)

// TableNote is one non-private note the row query is allowed to read.
type TableNote struct {
	ID          int64
	Title       string
	Description string
	ParentTitle string
	ParentSort  int
	SortOrder   int
}

// Head is one note the query may open. Depth 0 is the note itself, 1 is `+` / `+1`
// (its direct children too), n is n generations below it, HeadAll is every generation.
type Head struct {
	ID    int64
	Depth int
}

// HeadScope is the list of heads, one entry per id. A repeated id keeps the deeper reach.
type HeadScope struct {
	Heads []Head
}

// TableNoteLister returns notes under scope whose text mentions the table id.
// It hides private notes and does not execute inserts in their text.
type TableNoteLister func(context.Context, string, HeadScope) ([]TableNote, error)

type tableNotesKey struct{}
type hostNoteKey struct{}

// WithTableNotes attaches the workspace's note listing to an embedded run.
func WithTableNotes(ctx context.Context, list TableNoteLister) context.Context {
	return context.WithValue(ctx, tableNotesKey{}, list)
}

// WithHostNote is the note whose text launched the script, when the source has no id of its own.
func WithHostNote(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, hostNoteKey{}, id)
}

func hostNoteID(ctx context.Context) int64 {
	id, _ := ctx.Value(hostNoteKey{}).(int64)
	return id
}

func tableNoteLister(ctx context.Context) TableNoteLister {
	list, _ := ctx.Value(tableNotesKey{}).(TableNoteLister)
	return list
}

type notesByRowParams struct {
	Table string `json:"table"`
	Row   string `json:"row"`
	Heads string `json:"heads"`
}

var headToken = regexp.MustCompile(`^note=([1-9][0-9]*)(\+(?:!|[1-9][0-9]*)?)?$`)

// ParseHeads reads a list separated by commas, spaces or lines.
// `note=12` is that note. `note=12+` and `note=12+1` add its children, `note=12+2` two generations,
// `note=12+!` every generation under it. The same id keeps the deeper reach.
func ParseHeads(raw string) (HeadScope, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	if len(fields) == 0 {
		return HeadScope{}, fmt.Errorf("нужен список головных, например note=12+! или note=12+2")
	}
	if len(fields) > maxHeads {
		return HeadScope{}, fmt.Errorf("головных больше %d", maxHeads)
	}
	depthOf := map[int64]int{}
	var order []int64
	for _, field := range fields {
		m := headToken.FindStringSubmatch(field)
		if m == nil {
			return HeadScope{}, fmt.Errorf("головная %q: нужно note=12+! или note=12+2", field)
		}
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return HeadScope{}, fmt.Errorf("головная %q: слишком большой номер", field)
		}
		depth, err := headDepth(m[2])
		if err != nil {
			return HeadScope{}, fmt.Errorf("головная %q: %w", field, err)
		}
		if prev, ok := depthOf[id]; ok {
			depthOf[id] = deeperHead(prev, depth)
			continue
		}
		depthOf[id] = depth
		order = append(order, id)
	}
	scope := HeadScope{Heads: make([]Head, 0, len(order))}
	for _, id := range order {
		scope.Heads = append(scope.Heads, Head{ID: id, Depth: depthOf[id]})
	}
	return scope, nil
}

func headDepth(suffix string) (int, error) {
	switch suffix {
	case "":
		return 0, nil
	case "+", "+1":
		return 1, nil
	case "+!":
		return HeadAll, nil
	}
	n, err := strconv.Atoi(suffix[1:])
	if err != nil || n < 1 || n > maxHeadDepth {
		return 0, fmt.Errorf("глубина +! либо число поколений от 1 до %d", maxHeadDepth)
	}
	return n, nil
}

func deeperHead(a, b int) int {
	if a == HeadAll || b == HeadAll {
		return HeadAll
	}
	if a > b {
		return a
	}
	return b
}

func notesByRowPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p notesByRowParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if !tableKey.MatchString(p.Table) || !tableKey.MatchString(p.Row) {
		return nil, nil, fmt.Errorf("нужны table-id и row-id")
	}
	if _, err := ParseHeads(p.Heads); err != nil {
		return nil, nil, err
	}
	return []Port{req("ref", TRefNote)}, []Port{req("value", TStr), req("items", TDoc), req("errors", TDoc)}, nil
}

type rowHit struct {
	note TableNote
}

func runNotesByRow(ctx context.Context, rs *resolved, ins map[string][]Token) (map[string]Token, error) {
	var p notesByRowParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	scope, err := ParseHeads(p.Heads)
	if err != nil {
		return nil, err
	}
	ref, ok := ins["ref"][0].Value.(Ref)
	if !ok || ref.Kind != "note" || ref.ID <= 0 {
		return nil, fmt.Errorf("нужна заметка, с которой сравнивать строку")
	}
	list := tableNoteLister(ctx)
	if list == nil {
		return nil, fmt.Errorf("выборка заметок здесь недоступна")
	}
	notes, err := list(ctx, p.Table, scope)
	if err != nil {
		return nil, err
	}
	var hits []rowHit
	var broken []TableNote
	var reasons []string
	for _, n := range notes {
		if n.ID == ref.ID {
			continue
		}
		table, err := readMarkdownTable(n.Description, p.Table)
		if err != nil {
			if len(broken) < maxRowErrors {
				broken = append(broken, n)
				reasons = append(reasons, err.Error())
			}
			continue
		}
		cell := ""
		found := false
		for _, row := range table.Rows {
			if row.ID != p.Row {
				continue
			}
			found = true
			cell = strings.Join(row.Cells, "\n")
			break
		}
		if !found {
			continue
		}
		for _, r := range refs.Parse(cell) {
			if r.Kind == "note" && r.ID == ref.ID {
				hits = append(hits, rowHit{note: n})
				break
			}
		}
		if len(hits) > maxRowMatches {
			break
		}
	}
	extra := 0
	if len(hits) > maxRowMatches {
		extra = len(hits) - maxRowMatches
		hits = hits[:maxRowMatches]
	}
	value := formatRowHits(hits, extra, broken, reasons)
	if len(value) > MaxTextBytes {
		return nil, fmt.Errorf("список больше 64 КБ")
	}
	items := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		items = append(items, map[string]any{"id": h.note.ID, "title": h.note.Title})
	}
	errs := make([]map[string]any, 0, len(broken))
	for i, n := range broken {
		errs = append(errs, map[string]any{"id": n.ID, "title": n.Title, "error": reasons[i]})
	}
	token := func(v any) Token {
		raw, _ := json.Marshal(v)
		return Token{Type: TDoc, Value: json.RawMessage(raw)}
	}
	return map[string]Token{
		"value":  {Type: TStr, Value: value},
		"items":  token(items),
		"errors": token(errs),
	}, nil
}

func formatRowHits(hits []rowHit, extra int, broken []TableNote, reasons []string) string {
	var b strings.Builder
	if len(hits) == 0 {
		b.WriteString("Нет заметок с этой строкой.")
	} else {
		group := "\x00"
		for _, h := range hits {
			title := h.note.ParentTitle
			if title != group {
				if group != "\x00" {
					b.WriteByte('\n')
				}
				if title != "" {
					fmt.Fprintf(&b, "### %s\n\n", mdTitle(title))
				}
				group = title
			}
			fmt.Fprintf(&b, "- note=%d\n", h.note.ID)
		}
		if extra > 0 {
			fmt.Fprintf(&b, "\n… ещё %d\n", extra)
		}
	}
	if len(broken) > 0 {
		b.WriteString("\n### Не разобраны\n\n")
		for i, n := range broken {
			fmt.Fprintf(&b, "- note=%d — %s\n", n.ID, oneLine(reasons[i]))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func mdTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len([]rune(s)) > 180 {
		return string([]rune(s)[:180]) + "…"
	}
	return s
}
