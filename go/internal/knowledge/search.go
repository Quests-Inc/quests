package knowledge

import (
	"context"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/search"
)

// Legacy note-search response over the shared entity-search matcher.

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
	Filters []search.Filter
	Limit   int
	// ExcludePrivate leaves PRIVATE notes out (what an agent must never be shown).
	ExcludePrivate bool
	// Agent marks a search made for an agent: the hits count as "shown" (an impression, never interest).
	Agent bool
}

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
	maxQueryRunes      = 200
)

var wordRe = regexp.MustCompile(`[\p{L}\p{N}_]+`)

// SearchNotes finds notes matching q, best first.
func (s *Store) SearchNotes(ctx context.Context, q string, o SearchOpts) ([]SearchHit, error) {
	req := search.NormalizeNoteVars(search.Request{Query: q, Types: []string{"note"}, Limit: o.Limit, ExcludePrivate: o.ExcludePrivate, Filters: o.Filters})
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if req.Query == "" && !search.HasVarFilter(req.Filters) {
		return []SearchHit{}, nil
	}
	found, err := s.SearchEntities(ctx, req)
	if err != nil {
		return nil, err
	}
	page := search.Paginate(found, req).Hits
	ids := make([]int64, len(page))
	for i, h := range page {
		ids[i] = h.ID
	}
	notes, err := s.GetNotesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []SearchHit{}
	for _, h := range page {
		n, ok := notes[h.ID]
		if !ok {
			continue // the note vanished (or left the workspace) between match and fetch
		}
		out = append(out, SearchHit{ID: h.ID, Title: h.Title, Score: h.Score, Where: h.Where, Snippet: h.Snippet, ParentID: n.ParentID, UpdatedAt: n.UpdatedAt, CreatedAt: n.CreatedAt, Private: n.IsPrivate})
	}
	return s.rankByReading(ctx, out, o), nil
}

// rankByReading attaches reading metadata; ranking is already shared by every client in the provider.
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
	}

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

// GetNotesByIDs loads notes in one query, keyed by id. Notes the RLS/current
// workspace does not see are simply absent from the result.
func (s *Store) GetNotesByIDs(ctx context.Context, ids []int64) (map[int64]Note, error) {
	out := make(map[int64]Note, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ph := strings.Repeat("?,", len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+noteCols+` FROM note WHERE id IN (`+ph[:len(ph)-1]+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out[n.ID] = n
	}
	return out, rows.Err()
}

// SearchVariants and prefixTerm also serve in-document search.
func SearchVariants(q string) []string { return search.Variants(q) }
func prefixTerm(q string) string       { return search.Prefix(q) }
