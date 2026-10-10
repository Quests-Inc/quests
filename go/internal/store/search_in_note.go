package store

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

// InNoteResult is a search inside one note: the words that matched (as written in the text, so
// "деплоя" finds "деплой" and the client can mark them) and how many paragraphs carry a match.
type InNoteResult struct {
	Terms    []string `json:"terms"`
	Passages int      `json:"passages"`
}

const (
	maxNotePassages = 3000
	maxPassageRunes = 6000
	passageSep      = "\x01"
)

var markedRe = regexp.MustCompile(MarkOpen + `(.+?)` + MarkClose)

// notePassages cuts a note into paragraphs (blank-line separated); the search runs over them one by one.
func notePassages(body string) []string {
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(strings.ReplaceAll(body, passageSep, " "), -1) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if r := []rune(p); len(r) > maxPassageRunes {
			p = string(r[:maxPassageRunes])
		}
		out = append(out, p)
		if len(out) >= maxNotePassages {
			break
		}
	}
	return out
}

// SearchInNote applies the notes search's rules (word forms in Russian and English, a prefix of the
// last word, the other keyboard layout, "phrases", -exclusions) to the text of one note.
func (s *Store) SearchInNote(ctx context.Context, body, q string) (InNoteResult, error) {
	res := InNoteResult{Terms: []string{}}
	q = strings.TrimSpace(q)
	if r := []rune(q); len(r) > maxQueryRunes {
		q = string(r[:maxQueryRunes])
	}
	if q == "" || strings.TrimSpace(body) == "" {
		return res, nil
	}
	passages := notePassages(body)
	variants := SearchVariants(q)
	r, err := s.searchInNotePG(ctx, passages, q, variants)
	if err == nil {
		return r, nil
	}

	return searchInNotePlain(passages, variants), nil
}

func (s *Store) searchInNotePG(ctx context.Context, passages []string, q string, variants []string) (InNoteResult, error) {
	res := InNoteResult{Terms: []string{}}
	var tsParts, prefParts []string
	var args []any
	for _, v := range variants {
		tsParts = append(tsParts, `websearch_to_tsquery('russian', ?) || websearch_to_tsquery('english', ?)`)
		args = append(args, v, v)
	}
	for _, v := range variants {
		if p := prefixTerm(v); p != "" {
			prefParts = append(prefParts, `to_tsquery('russian', ?) || to_tsquery('english', ?)`)
			args = append(args, p, p)
		}
	}
	prefExpr := `''::tsquery`
	if len(prefParts) > 0 {
		prefExpr = strings.Join(prefParts, ` || `)
	}
	args = append(args, strings.Join(passages, passageSep))
	opts := `'StartSel=` + MarkOpen + `, StopSel=` + MarkClose + `, HighlightAll=true'`
	query := `
	WITH q AS (SELECT (` + strings.Join(tsParts, ` || `) + `) AS ts, (` + prefExpr + `) AS pq),
	p AS (SELECT t FROM unnest(string_to_array(?, chr(1))) AS u(t))
	SELECT ts_headline('russian', t, q.ts || q.pq, ` + opts + `), ts_headline('english', t, q.ts || q.pq, ` + opts + `)
	FROM p, q
	WHERE (to_tsvector('russian', t) || to_tsvector('english', t)) @@ (q.ts || q.pq)`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var ru, en string
		if err := rows.Scan(&ru, &en); err != nil {
			return res, err
		}
		res.Passages++
		for _, h := range []string{ru, en} {
			for _, m := range markedRe.FindAllStringSubmatch(h, -1) {
				w := strings.ToLower(strings.TrimSpace(m[1]))
				if w != "" && !seen[w] {
					seen[w] = true
					res.Terms = append(res.Terms, w)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return res, err
	}
	sort.Slice(res.Terms, func(i, j int) bool { return len(res.Terms[i]) > len(res.Terms[j]) })
	return res, nil
}

func searchInNotePlain(passages []string, variants []string) InNoteResult {
	res := InNoteResult{Terms: []string{}}
	var terms []string
	seen := map[string]bool{}
	for _, v := range variants {
		for _, w := range wordRe.FindAllString(strings.ToLower(v), -1) {
			if !seen[w] {
				seen[w] = true
				terms = append(terms, w)
			}
		}
	}
	found := map[string]bool{}
	for _, p := range passages {
		low, hit := strings.ToLower(p), false
		for _, t := range terms {
			if strings.Contains(low, t) {
				found[t], hit = true, true
			}
		}
		if hit {
			res.Passages++
		}
	}
	for _, t := range terms {
		if found[t] {
			res.Terms = append(res.Terms, t)
		}
	}
	return res
}
