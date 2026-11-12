// Package search defines one entity-search contract and PostgreSQL matcher.
// Contexts supply only their own projections; cross-context orchestration lives in store.
package search

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Filter struct {
	Kind   string   `json:"kind,omitempty"`
	Key    string   `json:"key"`
	Values []string `json:"values"`
	Not    bool     `json:"not,omitempty"`
	All    bool     `json:"all,omitempty"`
}
type Request struct {
	Query   string   `json:"q"`
	Types   []string `json:"types"`
	Filters []Filter `json:"filters,omitempty"`
	// Allowed is an optional explicit candidate scope (empty list means no candidates).
	Allowed        map[string][]int64 `json:"allowed,omitempty"`
	Excluded       map[string][]int64 `json:"excluded,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	Offset         int                `json:"offset,omitempty"`
	ExcludePrivate bool               `json:"-"`
}
type Hit struct {
	Kind       string  `json:"kind"`
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Where      string  `json:"where"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	ParentKind string  `json:"parent_kind,omitempty"`
	ParentID   *int64  `json:"parent_id,omitempty"`
}
type Page struct {
	Hits []Hit `json:"hits"`
	More bool  `json:"more"`
	Next int   `json:"next"`
}

// Source contains trusted SQL from the owning context, never from the HTTP caller.
type Source struct {
	Kind, From, ID, Title, Body, Where, ParentKind, ParentID string
	RankFactor                                               string // optional bounded domain relevance multiplier, applied before LIMIT
	// Fields are scalar expressions. Tests handle enumerated/boolean flags.
	Fields map[string]string
	Lists  map[string]string
	Tests  map[string]map[string]string
}

var Kinds = []string{"quest", "step", "questline", "note", "attachment", "template", "script", "hook", "event", "tag", "category", "runner", "job"}
var identity = regexp.MustCompile(`(?i)^(?:([a-z][a-z_-]*)\s*[=:]\s*)?([1-9][0-9]*)$`)

func Identity(q string) (string, int64, bool) {
	m := identity.FindStringSubmatch(strings.TrimSpace(q))
	if m == nil {
		return "", 0, false
	}
	id, err := strconv.ParseInt(m[2], 10, 64)
	return strings.ToLower(m[1]), id, err == nil
}
func (r *Request) Normalize() error {
	r.Query = strings.TrimSpace(r.Query)
	if len([]rune(r.Query)) > 200 {
		return fmt.Errorf("search query exceeds 200 characters")
	}
	if r.Types == nil {
		r.Types = []string{"quest", "step", "questline", "note", "attachment", "template", "script", "hook", "event"}
	}
	for _, k := range r.Types {
		if !slices.Contains(Kinds, k) {
			return fmt.Errorf("unknown search type %q", k)
		}
	}
	if r.Limit <= 0 {
		r.Limit = 30
	}
	if r.Limit > 100 {
		r.Limit = 100
	}
	if r.Offset < 0 || r.Offset > 100000 {
		return fmt.Errorf("negative search offset")
	}
	if len(r.Filters) > 32 {
		return fmt.Errorf("too many search filters")
	}
	for _, f := range r.Filters {
		if len(f.Values) > 100 {
			return fmt.Errorf("too many filter values")
		}
	}
	return nil
}
func Sort(h []Hit) {
	sort.SliceStable(h, func(i, j int) bool {
		a, b := h[i], h[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
}
func Paginate(h []Hit, r Request) Page {
	Sort(h)
	start := min(r.Offset, len(h))
	end := min(start+r.Limit, len(h))
	return Page{Hits: h[start:end], More: end < len(h), Next: end}
}

// Variants is shared by every entity type (including legacy note-search clients).
func Variants(q string) []string {
	const en = "qwertyuiop[]asdfghjkl;'zxcvbnm,."
	const ru = "йцукенгшщзхъфывапролджэячсмитьбю"
	var latin, cyr bool
	for _, r := range q {
		latin = latin || unicode.Is(unicode.Latin, r)
		cyr = cyr || unicode.Is(unicode.Cyrillic, r)
	}
	from, to := []rune(en), []rune(ru)
	if cyr && !latin {
		from, to = to, from
	}
	if latin == cyr {
		return []string{q}
	}
	m := map[rune]rune{}
	for i, r := range from {
		m[r] = to[i]
	}
	mapped := strings.Map(func(r rune) rune {
		if v, ok := m[unicode.ToLower(r)]; ok {
			return v
		}
		return r
	}, q)
	if mapped == q {
		return []string{q}
	}
	return []string{q, mapped}
}

var lastWord = regexp.MustCompile(`[\p{L}\p{N}_]+$`)

func Prefix(q string) string {
	w := lastWord.FindString(q)
	if len([]rune(w)) < 3 || strings.Contains(q, `"`) || strings.HasSuffix(strings.TrimSuffix(q, w), "-") || strings.EqualFold(w, "or") {
		return ""
	}
	return strings.ToLower(w) + ":*"
}

func Query(ctx context.Context, db *sql.DB, s Source, r Request) ([]Hit, error) {
	if !slices.Contains(r.Types, s.Kind) {
		return []Hit{}, nil
	}
	typed, id, isID := Identity(r.Query)
	if isID && typed != "" && typed != s.Kind {
		return []Hit{}, nil
	}
	if r.ExcludePrivate && s.Kind == "note" {
		s.Where = "(" + s.Where + ") AND n.is_private=0"
	}
	where := []string{s.Where}
	args := []any{}
	// Constrain the source before matching/ranking/limiting.
	addIDs := func(ids []int64, neg bool) {
		if len(ids) == 0 {
			if !neg {
				where = append(where, "FALSE")
			}
			return
		}
		p := make([]string, len(ids))
		for i, v := range ids {
			p[i] = "?"
			args = append(args, v)
		}
		op := " IN "
		if neg {
			op = " NOT IN "
		}
		where = append(where, s.ID+op+"("+strings.Join(p, ",")+")")
	}
	if ids, ok := r.Allowed[s.Kind]; ok {
		addIDs(ids, false)
	}
	addIDs(r.Excluded[s.Kind], true)
	for _, f := range r.Filters {
		if f.Kind != "" && f.Kind != s.Kind {
			continue
		}
		parts := []string{}
		for _, v := range f.Values {
			if test, ok := s.Tests[f.Key][v]; ok {
				parts = append(parts, "("+test+")")
				continue
			}
			if list, ok := s.Lists[f.Key]; ok {
				parts = append(parts, "?=ANY("+list+")")
				args = append(args, v)
				continue
			}
			field, ok := s.Fields[f.Key]
			if !ok {
				parts = append(parts, "FALSE")
				continue
			}
			parts = append(parts, "COALESCE(("+field+")::text,'none')=?")
			args = append(args, v)
		}
		if len(parts) == 0 {
			continue
		}
		op := " OR "
		if f.All {
			op = " AND "
		}
		test := "(" + strings.Join(parts, op) + ")"
		if f.Not {
			test = "NOT " + test
		}
		where = append(where, test)
	}
	factor := "1.0"
	if s.RankFactor != "" {
		factor = s.RankFactor
	}
	parent := "NULL::bigint"
	if s.ParentID != "" {
		parent = s.ParentID
	}
	// Match expressions use bound values in a CTE so each variant is parsed once per request.
	variants := Variants(r.Query)
	valueRows := []string{}
	inputArgs := []any{}
	for _, v := range variants {
		valueRows = append(valueRows, "(?::text,?::text)")
		inputArgs = append(inputArgs, v, Prefix(v))
	}
	title := "COALESCE(" + s.Title + ",'')"
	body := "COALESCE(" + s.Body + ",'')"
	sqlText := `WITH variants(raw,pref) AS (VALUES ` + strings.Join(valueRows, ",") + `),
 terms AS (SELECT raw, (websearch_to_tsquery('russian',raw)||websearch_to_tsquery('english',raw)||CASE WHEN pref='' THEN ''::tsquery ELSE to_tsquery('russian',pref)||to_tsquery('english',pref) END) AS ts FROM variants),
 candidates AS (SELECT ` + s.ID + ` AS id,` + title + ` AS title,` + body + ` AS body,` + parent + ` AS parent_id,` + factor + ` AS factor FROM ` + s.From + ` WHERE ` + strings.Join(where, " AND ") + `),
 matched AS (SELECT c.*, CASE WHEN id=? AND ?::int=1 THEN 10000
 WHEN ?::int=1 THEN 1
 WHEN EXISTS(SELECT 1 FROM terms t WHERE strpos(lower(c.title),lower(t.raw))>0 OR (to_tsvector('russian',c.title)||to_tsvector('english',c.title))@@t.ts) THEN 100
 WHEN EXISTS(SELECT 1 FROM terms t WHERE strpos(lower(c.body),lower(t.raw))>0 OR (to_tsvector('russian',left(c.body,300000))||to_tsvector('english',left(c.body,300000)))@@t.ts) THEN 10
 WHEN EXISTS(SELECT 1 FROM terms t WHERE length(t.raw)>=4 AND t.raw !~ '["-]' AND lower(t.raw) NOT LIKE '% or %' AND word_similarity(lower(t.raw),lower(c.title))>0.45) THEN 1
 ELSE 0 END AS score FROM candidates c)
 SELECT id,title,parent_id,CASE WHEN score IN (10,100) THEN ts_headline('russian',left(body,20000),(SELECT ts FROM terms LIMIT 1),'StartSel=⟪, StopSel=⟫, MaxWords=26, MinWords=8, MaxFragments=2') ELSE left(body,200) END,score*CASE WHEN score IN(1,10000) THEN 1.0 ELSE factor END AS relevance,CASE score WHEN 10000 THEN 'id' WHEN 100 THEN 'title' WHEN 10 THEN 'body' ELSE 'fuzzy' END FROM matched WHERE score>0`
	allArgs := append(inputArgs, args...)
	allArgs = append(allArgs, id, isID, r.Query == "")
	if isID && typed != "" {
		sqlText += " AND id=?"
		allArgs = append(allArgs, id)
	}
	sqlText += ` ORDER BY relevance DESC,title COLLATE "C",id LIMIT ?`
	allArgs = append(allArgs, r.Offset+r.Limit+1)
	rows, err := db.QueryContext(ctx, sqlText, allArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hit{}
	for rows.Next() {
		h := Hit{Kind: s.Kind, ParentKind: s.ParentKind}
		if err := rows.Scan(&h.ID, &h.Title, &h.ParentID, &h.Snippet, &h.Score, &h.Where); err != nil {
			return nil, err
		}

		if r.Query == "" {
			h.Where = "title"
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
