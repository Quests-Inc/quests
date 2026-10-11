package knowledge

import (
	"context"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// Search over notes (quest=354). On PostgreSQL: the generated tsvector (Russian and English
// stemming, title above body), ts_rank_cd for the order, ts_headline for the snippet, prefix
// matching on the last word, keyboard-layout variants and pg_trgm for typos in titles.
// When that fails (an extension missing, a schema mid-migration): a plain scorer over the loaded notes, same result shape.

// Snippet marks around a matched word (private-use-like brackets that do not occur in prose);
// the client escapes the text first and only then turns the marks into highlighting.
const (
	MarkOpen  = "⟪"
	MarkClose = "⟫"
)

// SearchHit is one found note: what, how well, where it matched and a fragment to show.
type SearchHit struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Score     float64   `json:"score"`
	Where     string    `json:"where"` // title | body | fuzzy
	Snippet   string    `json:"snippet"`
	ParentID  *int64    `json:"parent_id"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
	Private   bool      `json:"is_private"`
	// Outdated: the note's own check says it has drifted from its source (quest=370); Reason says how.
	Outdated       bool   `json:"outdated"`
	OutdatedReason string `json:"outdated_reason,omitempty"`
	// Popularity says why reading history moved this hit up or down (see notestats.go).
	Popularity *NotePopularity `json:"popularity,omitempty"`
}

// SearchOpts narrows a search.
type SearchOpts struct {
	Limit int
	// ExcludePrivate leaves PRIVATE notes out (what an agent must never be shown).
	ExcludePrivate bool
	// Agent marks a search made for an agent: the hits count as "shown" (an impression, never interest).
	Agent bool
}

// outdatedDemotion: a note whose own check says it is stale ranks below a fresh one of the same relevance.
const outdatedDemotion = 0.8

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
	maxQueryRunes      = 200
)

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// SearchNotes finds notes matching q, best first.
func (s *Store) SearchNotes(ctx context.Context, q string, o SearchOpts) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if r := []rune(q); len(r) > maxQueryRunes {
		q = string(r[:maxQueryRunes])
	}
	if q == "" {
		return []SearchHit{}, nil
	}
	if o.Limit <= 0 {
		o.Limit = defaultSearchLimit
	}
	if o.Limit > maxSearchLimit {
		o.Limit = maxSearchLimit
	}
	variants := SearchVariants(q)
	var hits []SearchHit
	var err error
	hits, err = s.searchNotesPG(ctx, q, variants, o)
	if err != nil {
		log.Printf("search: postgres search failed, falling back to the plain scorer: %v", err)
		hits, err = s.searchNotesPlain(ctx, variants, o)
	}

	if err != nil {
		return nil, err
	}
	return s.rankByReading(ctx, hits, o), nil
}

// rankByReading nudges relevance by reading history (bounded, see notestats.go) and re-sorts.
// Typo-only matches stay behind the real ones whatever their history.
func (s *Store) rankByReading(ctx context.Context, hits []SearchHit, o SearchOpts) []SearchHit {
	if len(hits) == 0 {
		return hits
	}
	born := make(map[int64]time.Time, len(hits))
	for _, h := range hits {
		born[h.ID] = h.CreatedAt
	}
	if checks, err := s.NoteChecks(ctx); err == nil {
		for i := range hits {
			if c, ok := checks[hits[i].ID]; ok && c.Outdated {
				hits[i].Outdated, hits[i].OutdatedReason = true, c.Reason
			}
		}
	}
	pop, err := s.Popularity(ctx, born)
	if err != nil {
		log.Printf("search: reading history unavailable: %v", err)
		return hits
	}
	for i := range hits {
		p := pop[hits[i].ID]
		hits[i].Popularity = &p
		if hits[i].Where != "fuzzy" {
			hits[i].Score *= 1 + p.Boost
			if hits[i].Outdated {
				hits[i].Score *= outdatedDemotion // still found, just not first
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if (hits[i].Where == "fuzzy") != (hits[j].Where == "fuzzy") {
			return hits[j].Where == "fuzzy"
		}
		return hits[i].Score > hits[j].Score
	})
	if o.Agent {
		ids := make([]int64, len(hits))
		for i, h := range hits {
			ids[i] = h.ID
		}
		if err := s.RecordShown(ctx, ids); err != nil {
			log.Printf("search: recording impressions: %v", err)
		}
	}
	return hits
}

// SearchVariants is the query itself plus, when it is typed in one script, the same keys read in
// the other layout ("ghbdtn" → "привет"): somebody searching with the wrong layout still finds it.
func SearchVariants(q string) []string {
	out := []string{q}
	var latin, cyr int
	for _, r := range q {
		switch {
		case unicode.Is(unicode.Latin, r):
			latin++
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		}
	}
	switch {
	case latin > 0 && cyr == 0:
		if v := mapLayout(q, enToRu); v != q {
			out = append(out, v)
		}
	case cyr > 0 && latin == 0:
		if v := mapLayout(q, ruToEn); v != q {
			out = append(out, v)
		}
	}
	return out
}

const enRuKeys = "qwertyuiop[]asdfghjkl;'zxcvbnm,."
const ruEnKeys = "йцукенгшщзхъфывапролджэячсмитьбю"

var enToRu, ruToEn = layoutMap(enRuKeys, ruEnKeys), layoutMap(ruEnKeys, enRuKeys)

func layoutMap(from, to string) map[rune]rune {
	f, t := []rune(from), []rune(to)
	m := make(map[rune]rune, len(f))
	for i := range f {
		m[f[i]] = t[i]
	}
	return m
}

func mapLayout(s string, m map[rune]rune) string {
	return strings.Map(func(r rune) rune {
		if v, ok := m[unicode.ToLower(r)]; ok {
			return v
		}
		return r
	}, s)
}

// prefixTerm is the last typed word as a prefix term ("депл" → "депл:*"), or "" when it is too short
// or not a plain word.
func prefixTerm(q string) string {
	words := wordRe.FindAllString(q, -1)
	if len(words) == 0 {
		return ""
	}
	last := strings.ToLower(words[len(words)-1])
	if len([]rune(last)) < 3 || !strings.HasSuffix(q, words[len(words)-1]) {
		return "" // the last word is finished (followed by punctuation) or too short to be a prefix
	}
	// A word that is excluded ("-борщ"), part of a phrase or an operator is not a prefix to complete.
	if strings.ContainsAny(q, `"`) || strings.HasSuffix(strings.TrimSuffix(q, words[len(words)-1]), "-") ||
		strings.EqualFold(last, "or") {
		return ""
	}
	return last + ":*"
}

func (s *Store) searchNotesPG(ctx context.Context, q string, variants []string, o SearchOpts) ([]SearchHit, error) {
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
	priv := ""
	if o.ExcludePrivate {
		priv = ` AND n.is_private = 0`
	}
	query := `
	WITH q AS (SELECT (` + strings.Join(tsParts, ` || `) + `) AS ts, (` + prefExpr + `) AS pq),
	hit AS (
		SELECT n.id, n.title, n.description, n.parent_id, n.updated_at, n.created_at, n.is_private,
		       ts_rank_cd(n.search_tsv, q.ts) + 0.3 * ts_rank_cd(n.search_tsv, q.pq) AS rank,
		       ((to_tsvector('russian', n.title) || to_tsvector('english', n.title)) @@ (q.ts || q.pq)) AS in_title,
		       (q.ts || q.pq) AS tq
		FROM note n, q
		WHERE n.search_tsv @@ (q.ts || q.pq)` + priv + `
		ORDER BY rank DESC, n.updated_at DESC
		LIMIT ?
	)
	SELECT id, title, parent_id, updated_at, created_at, is_private, rank, in_title,
	       ts_headline('russian', left(description, 20000), tq, 'StartSel=` + MarkOpen + `, StopSel=` + MarkClose + `, MaxWords=26, MinWords=8, MaxFragments=2, FragmentDelimiter= … '),
	       ts_headline('english', left(description, 20000), tq, 'StartSel=` + MarkOpen + `, StopSel=` + MarkClose + `, MaxWords=26, MinWords=8, MaxFragments=2, FragmentDelimiter= … '),
	       left(description, 200)
	FROM hit ORDER BY rank DESC, updated_at DESC`
	args = append(args, o.Limit)

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var parent *int64
		var updated, created string
		var priv2 int
		var inTitle bool
		var hRu, hEn, head string
		if err := rows.Scan(&h.ID, &h.Title, &parent, &updated, &created, &priv2, &h.Score, &inTitle, &hRu, &hEn, &head); err != nil {
			return nil, err
		}
		h.ParentID, h.Private = parent, priv2 != 0
		h.UpdatedAt, _ = timeutil.ParseFlexible(updated)
		h.CreatedAt, _ = timeutil.ParseFlexible(created)
		h.Where = "body"
		if inTitle {
			h.Where = "title"
		}
		h.Snippet = pickSnippet(hRu, hEn, head)
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(hits) < 5 && len(hits) < o.Limit {
		extra, err := s.fuzzyTitles(ctx, q, o, hits)
		if err != nil {
			return nil, err
		}
		hits = append(hits, extra...)
	}
	if o.Limit < len(hits) {
		hits = hits[:o.Limit]
	}
	return hits, nil
}

// pickSnippet prefers a fragment that actually shows a match; else the start of the body.
func pickSnippet(ru, en, head string) string {
	for _, h := range []string{ru, en} {
		if strings.Contains(h, MarkOpen) {
			return strings.TrimSpace(h)
		}
	}
	return strings.TrimSpace(head)
}

// fuzzyTitles adds notes whose title is close to the query (a typo), after the real matches.
func (s *Store) fuzzyTitles(ctx context.Context, q string, o SearchOpts, have []SearchHit) ([]SearchHit, error) {
	priv := ""
	if o.ExcludePrivate {
		priv = ` AND is_private = 0`
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, title, parent_id, updated_at, created_at, is_private, left(description, 200),
		       word_similarity(lower(?), lower(title)) AS sim
		FROM note
		WHERE word_similarity(lower(?), lower(title)) > 0.45`+priv+`
		ORDER BY sim DESC LIMIT ?`, q, q, o.Limit+len(have))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for _, h := range have {
		seen[h.ID] = true
	}
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		var parent *int64
		var updated, created string
		var priv2 int
		var sim float64
		if err := rows.Scan(&h.ID, &h.Title, &parent, &updated, &created, &priv2, &h.Snippet, &sim); err != nil {
			return nil, err
		}
		if seen[h.ID] {
			continue
		}
		h.ParentID, h.Private, h.Where, h.Score = parent, priv2 != 0, "fuzzy", sim*0.05
		h.UpdatedAt, _ = timeutil.ParseFlexible(updated)
		h.CreatedAt, _ = timeutil.ParseFlexible(created)
		out = append(out, h)
	}
	return out, rows.Err()
}

// searchNotesPlain scores notes in memory: a word in the title counts most, then in the body.
func (s *Store) searchNotesPlain(ctx context.Context, variants []string, o SearchOpts) ([]SearchHit, error) {
	notes, err := s.ListNotes(ctx, NoteFilter{})
	if err != nil {
		return nil, err
	}
	var terms []string
	for _, v := range variants {
		terms = append(terms, wordRe.FindAllString(strings.ToLower(v), -1)...)
	}
	hits := []SearchHit{}
	for _, n := range notes {
		if o.ExcludePrivate && n.IsPrivate {
			continue
		}
		title, body := strings.ToLower(n.Title), strings.ToLower(n.Description)
		score, inTitle := 0.0, false
		first := -1
		for _, t := range terms {
			if strings.Contains(title, t) {
				score += 3
				inTitle = true
			}
			if c := strings.Count(body, t); c > 0 {
				score += math.Min(float64(c), 5) * 0.2
				if i := strings.Index(body, t); first < 0 || i < first {
					first = i
				}
			}
		}
		if score == 0 {
			continue
		}
		h := SearchHit{ID: n.ID, Title: n.Title, Score: score, Where: "body", ParentID: n.ParentID, UpdatedAt: n.UpdatedAt, CreatedAt: n.CreatedAt, Private: n.IsPrivate}
		if inTitle {
			h.Where = "title"
		}
		h.Snippet = plainSnippet(n.Description, terms, first)
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].UpdatedAt.After(hits[j].UpdatedAt)
	})
	if len(hits) > o.Limit {
		hits = hits[:o.Limit]
	}
	return hits, nil
}

// plainSnippet cuts a window around the first match and marks every matched term in it.
func plainSnippet(body string, terms []string, firstLower int) string {
	runes := []rune(body)
	start := 0
	if firstLower > 60 {
		// firstLower is a byte offset into the lowercased text; counting runes on the original is close enough
		start = len([]rune(strings.ToLower(body)[:firstLower])) - 60
	}
	if start < 0 {
		start = 0
	}
	end := start + 200
	if end > len(runes) {
		end = len(runes)
	}
	frag := string(runes[start:end])
	for _, t := range terms {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(t))
		frag = re.ReplaceAllStringFunc(frag, func(m string) string { return MarkOpen + m + MarkClose })
	}
	if start > 0 {
		frag = "… " + frag
	}
	return strings.TrimSpace(frag)
}
