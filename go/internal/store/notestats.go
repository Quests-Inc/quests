package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// Reading signals for ranking notes (quest=353).
//
// What counts: a person opening a note (weight 1) and an agent reading one (0.3); an explicit
// "useful" (+2) or "not useful" (−3) beats both. What does not: being listed (counted apart as
// `shown`, never as interest), background clients (only the web UI and MCP report reads), repeated
// opens inside ReadDedupe. The score decays (half-life 30 days, applied lazily), is squashed by a
// logarithm, and only nudges a relevance-ranked list by at most +30%: popularity never outvotes
// relevance, and a note nobody has opened yet is not buried (a newness bonus fades over two weeks).

const (
	HalfLifeDays   = 30.0
	weightHuman    = 1.0
	weightAgent    = 0.3
	weightUp       = 2.0
	weightDown     = -3.0
	scoreCap       = 20.0 // a score at this level saturates the nudge
	popularityMax  = 0.20 // largest boost from reading history
	noveltyMax     = 0.10 // largest boost for a brand-new note
	noveltyHalfLfe = 14.0
	manualStep     = 0.05 // per manual weight point (−2..+2)
)

// NotePopularity is what the ranking and the agent get to see about a note.
type NotePopularity struct {
	Score       float64    `json:"score"` // decayed, as of now
	OpensHuman  int        `json:"opens_human"`
	OpensAgent  int        `json:"opens_agent"`
	Shown       int        `json:"shown"`
	LastReadAt  *time.Time `json:"last_read_at"`
	RatedUp     int        `json:"rated_up"`
	RatedDown   int        `json:"rated_down"`
	Manual      int        `json:"manual"`
	Boost       float64    `json:"boost"` // the multiplier on relevance minus 1 (0 … 0.3)
	Explanation string     `json:"why"`
}

// Decay returns score as of now given when it was last written.
func Decay(score float64, at, now time.Time) float64 {
	if score <= 0 || at.IsZero() || !now.After(at) {
		return math.Max(score, 0)
	}
	days := now.Sub(at).Hours() / 24
	return score * math.Pow(0.5, days/HalfLifeDays)
}

// Boost is the multiplier-minus-one that popularity, newness and a manual weight add to relevance.
func Boost(score float64, created, now time.Time, manual int) float64 {
	pop := 0.0
	if score > 0 {
		pop = math.Min(math.Log1p(score)/math.Log1p(scoreCap), 1)
	}
	age := math.Max(now.Sub(created).Hours()/24, 0)
	novelty := math.Pow(0.5, age/noveltyHalfLfe)
	b := popularityMax*pop + noveltyMax*novelty + manualStep*float64(clampInt(manual, -2, 2))
	return math.Max(-0.10, math.Min(b, 0.30))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type statRow struct {
	score               float64
	scoreAt, human, ag  sql.NullString
	opensH, opensA, shn int
	up, down, manual    int
}

func (s *Store) loadStat(ctx context.Context, noteID int64) (statRow, bool, error) {
	var r statRow
	err := s.DB.QueryRowContext(ctx, `SELECT score, score_at, last_human_at, last_agent_at, opens_human, opens_agent, shown, rated_up, rated_down, manual
		FROM note_stat WHERE note_id = ?`, noteID).Scan(&r.score, &r.scoreAt, &r.human, &r.ag, &r.opensH, &r.opensA, &r.shn, &r.up, &r.down, &r.manual)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

func parseTime(n sql.NullString) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	t, _ := timeutil.ParseFlexible(n.String)
	return t
}

// RecordNoteOpen counts a read: agent=false for a person in the web UI, true for an agent.
// A second read of the same kind inside ReadDedupe changes nothing. Returns whether it counted.
func (s *Store) RecordNoteOpen(ctx context.Context, noteID int64, agent bool) (bool, error) {
	now := timeutil.NowUTC()
	r, _, err := s.loadStat(ctx, noteID)
	if err != nil {
		return false, err
	}
	last := parseTime(r.human)
	w := weightHuman
	if agent {
		last, w = parseTime(r.ag), weightAgent
	}
	if !last.IsZero() && now.Sub(last) < ReadDedupe {
		return false, nil
	}
	eff := Decay(r.score, parseTime(r.scoreAt), now) + w
	col, cnt := "last_human_at", "opens_human"
	if agent {
		col, cnt = "last_agent_at", "opens_agent"
	}
	ts := timeutil.ToDBUTC(now)
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO note_stat (note_id, score, score_at, `+col+`, `+cnt+`) VALUES (?, ?, ?, ?, 1)
		ON CONFLICT (note_id) DO UPDATE SET score = excluded.score, score_at = excluded.score_at,
			`+col+` = excluded.`+col+`, `+cnt+` = note_stat.`+cnt+` + 1`, noteID, eff, ts, ts)
	return err == nil, err
}

// RecordShown counts that notes were put in front of an agent (never as interest).
func (s *Store) RecordShown(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO note_stat (note_id, shown) VALUES (?, 1)
			ON CONFLICT (note_id) DO UPDATE SET shown = note_stat.shown + 1`, id); err != nil {
			return err
		}
	}
	return nil
}

// RateNote records an explicit judgement of a note.
func (s *Store) RateNote(ctx context.Context, noteID int64, useful bool) error {
	now := timeutil.NowUTC()
	r, _, err := s.loadStat(ctx, noteID)
	if err != nil {
		return err
	}
	w, upCol := weightUp, "rated_up"
	if !useful {
		w, upCol = weightDown, "rated_down"
	}
	eff := math.Max(Decay(r.score, parseTime(r.scoreAt), now)+w, 0)
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO note_stat (note_id, score, score_at, `+upCol+`) VALUES (?, ?, ?, 1)
		ON CONFLICT (note_id) DO UPDATE SET score = excluded.score, score_at = excluded.score_at,
			`+upCol+` = note_stat.`+upCol+` + 1`, noteID, eff, timeutil.ToDBUTC(now))
	return err
}

// SetNoteWeight is the manual lever (−2 … +2): a note you know matters, or one that misleads.
func (s *Store) SetNoteWeight(ctx context.Context, noteID int64, weight int) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO note_stat (note_id, manual) VALUES (?, ?)
		ON CONFLICT (note_id) DO UPDATE SET manual = excluded.manual`, noteID, clampInt(weight, -2, 2))
	return err
}

// ResetNoteStats forgets everything counted for a note (a mistaken streak of opens, a stale favourite).
func (s *Store) ResetNoteStats(ctx context.Context, noteID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM note_stat WHERE note_id = ?`, noteID)
	return err
}

// Popularity returns the explained signals for the given notes (missing = never read).
func (s *Store) Popularity(ctx context.Context, created map[int64]time.Time) (map[int64]NotePopularity, error) {
	out := make(map[int64]NotePopularity, len(created))
	now := timeutil.NowUTC()
	for id, born := range created {
		p := NotePopularity{}
		r, ok, err := s.loadStat(ctx, id)
		if err != nil {
			return nil, err
		}
		if ok {
			p.Score = Decay(r.score, parseTime(r.scoreAt), now)
			p.OpensHuman, p.OpensAgent, p.Shown = r.opensH, r.opensA, r.shn
			p.RatedUp, p.RatedDown, p.Manual = r.up, r.down, r.manual
			last := parseTime(r.human)
			if a := parseTime(r.ag); a.After(last) {
				last = a
			}
			if !last.IsZero() {
				p.LastReadAt = &last
			}
		}

		p.Boost = Boost(p.Score, born, now, p.Manual)
		p.Explanation = explainBoost(p, born, now)
		out[id] = p
	}
	return out, nil
}

func explainBoost(p NotePopularity, born, now time.Time) string {
	switch {
	case p.Manual != 0:
		return "ручной вес заметки"
	case p.Score > 0.5 && p.RatedDown > p.RatedUp:
		return "читали, но оценили как неполезную"
	case p.Score > 0.5:
		return "её недавно читали"
	case now.Sub(born) < 7*24*time.Hour:
		return "новая заметка"
	}
	return ""
}
