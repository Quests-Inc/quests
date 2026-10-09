package store

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDecayHalvesEveryThirtyDays(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	if got := Decay(8, now.AddDate(0, 0, -30), now); math.Abs(got-4) > 1e-9 {
		t.Fatalf("after one half-life: %v", got)
	}
	if got := Decay(8, now.AddDate(0, 0, -90), now); math.Abs(got-1) > 1e-9 {
		t.Fatalf("after three: %v", got)
	}
	if Decay(-3, now.AddDate(0, 0, -1), now) != 0 || Decay(5, time.Time{}, now) != 5 {
		t.Fatal("negative scores clamp to 0; an unknown time keeps the score")
	}
}

// Reading history may only nudge: never past +30%, and a runaway score gains nothing more.
func TestBoostIsBoundedAndSaturates(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	old := now.AddDate(-10, 0, 0)
	if b := Boost(0, old, now, 0); b > 1e-6 {
		t.Fatalf("an old note nobody read must not move: %v", b)
	}
	mid, huge := Boost(20, old, now, 0), Boost(1e9, old, now, 0)
	if math.Abs(mid-popularityMax) > 1e-6 || math.Abs(huge-popularityMax) > 1e-6 {
		t.Fatalf("saturation: score 20 -> %v, score 1e9 -> %v (both must equal %v)", mid, huge, popularityMax)
	}
	if b := Boost(1e9, now, now, 2); b > 0.30+1e-9 {
		t.Fatalf("boost %v passes the +30%% ceiling", b)
	}
	if fresh, stale := Boost(0, now, now, 0), Boost(0, now.AddDate(0, 0, -60), now, 0); fresh <= stale {
		t.Fatalf("a new note should get more than an old one: %v vs %v", fresh, stale)
	}
	if b := Boost(0, old, now, -2); b >= 0 || b < -0.10-1e-9 {
		t.Fatalf("a manual downgrade is a small, bounded penalty: %v", b)
	}
}

const noteStatTestSchema = `
CREATE TABLE note (id INTEGER PRIMARY KEY, title TEXT);
INSERT INTO note (id, title) VALUES (1, 'a'), (2, 'b');
CREATE TABLE note_stat (
	note_id INTEGER PRIMARY KEY REFERENCES note(id) ON DELETE CASCADE,
	score REAL NOT NULL DEFAULT 0, score_at TEXT,
	opens_human INTEGER NOT NULL DEFAULT 0, opens_agent INTEGER NOT NULL DEFAULT 0, shown INTEGER NOT NULL DEFAULT 0,
	last_human_at TEXT, last_agent_at TEXT,
	rated_up INTEGER NOT NULL DEFAULT 0, rated_down INTEGER NOT NULL DEFAULT 0, manual INTEGER NOT NULL DEFAULT 0);
`

func openStatDB(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:stat_"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(noteStatTestSchema); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db, PGURL: "stub"} // PGURL only switches the feature on (SQLite runs the same SQL)
}

func TestReadSignalsDedupeAndWeigh(t *testing.T) {
	st := openStatDB(t)
	ctx := context.Background()
	born := map[int64]time.Time{1: time.Now().AddDate(-1, 0, 0), 2: time.Now().AddDate(-1, 0, 0)}
	pop := func(id int64) NotePopularity {
		m, err := st.Popularity(ctx, born)
		if err != nil {
			t.Fatal(err)
		}
		return m[id]
	}

	if counted, _ := st.RecordNoteOpen(ctx, 1, false); !counted {
		t.Fatal("the first open must count")
	}
	if counted, _ := st.RecordNoteOpen(ctx, 1, false); counted {
		t.Fatal("a second open inside the dedupe window counted again")
	}
	if p := pop(1); p.OpensHuman != 1 || math.Abs(p.Score-weightHuman) > 0.01 {
		t.Fatalf("after two human opens: %+v", p)
	}
	// An agent's read is worth less, and its own window is separate from the person's.
	if counted, _ := st.RecordNoteOpen(ctx, 1, true); !counted {
		t.Fatal("an agent read must count")
	}
	if p := pop(1); p.OpensAgent != 1 || math.Abs(p.Score-(weightHuman+weightAgent)) > 0.01 {
		t.Fatalf("after an agent read: %+v", p)
	}
	// Being listed is an impression, never interest.
	if err := st.RecordShown(ctx, []int64{2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if p := pop(2); p.Shown != 3 || p.Score != 0 {
		t.Fatalf("impressions leaked into the score: %+v", p)
	}
	// Explicit ratings beat opens; the score never goes below zero.
	_ = st.RateNote(ctx, 2, true)
	if p := pop(2); math.Abs(p.Score-weightUp) > 0.01 || p.RatedUp != 1 {
		t.Fatalf("after a thumbs-up: %+v", p)
	}
	_ = st.RateNote(ctx, 2, false)
	_ = st.RateNote(ctx, 2, false)
	if p := pop(2); p.Score != 0 || p.RatedDown != 2 {
		t.Fatalf("score under zero: %+v", p)
	}
	// Manual weight and reset.
	_ = st.SetNoteWeight(ctx, 1, 9)
	if p := pop(1); p.Manual != 2 {
		t.Fatalf("manual weight must clamp to 2: %+v", p)
	}
	if err := st.ResetNoteStats(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if p := pop(1); p.Score != 0 || p.OpensHuman != 0 || p.Manual != 0 || p.LastReadAt != nil {
		t.Fatalf("reset left traces: %+v", p)
	}
}

// Without the feature (no PostgreSQL) every call is a harmless no-op.
func TestReadSignalsOffWithoutPostgres(t *testing.T) {
	st := &Store{}
	ctx := context.Background()
	if c, err := st.RecordNoteOpen(ctx, 1, false); c || err != nil {
		t.Fatalf("%v %v", c, err)
	}
	if st.RateNote(ctx, 1, true) != nil || st.SetNoteWeight(ctx, 1, 1) != nil || st.ResetNoteStats(ctx, 1) != nil || st.RecordShown(ctx, []int64{1}) != nil {
		t.Fatal("no-ops must not fail")
	}
}
