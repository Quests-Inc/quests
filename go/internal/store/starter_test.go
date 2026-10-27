package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func starterStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	db, url := pgtest.NewWithURL(t)
	return New(db, url, nil), wsctx.With(context.Background(), 1)
}
func seedEntity(t *testing.T, s *Store, ctx context.Context, key string) int64 {
	t.Helper()
	var id int64
	if err := s.DB.QueryRowContext(ctx, `SELECT entity_id FROM seed_item WHERE key=?`, key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestStarterRepeatUpdateAndPreserve(t *testing.T) {
	s, ctx := starterStore(t)
	// Existing content and its references must survive seeding into a nonempty workspace.
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO note(id,title,description,created_at,updated_at) VALUES (700,'mine','note=700 script=999',now(),now())`); err != nil {
		t.Fatal(err)
	}
	pgtest.FixSequences(t, s.DatabaseURL)
	r, err := s.SeedStarter(ctx)
	if err != nil || r.Added == 0 {
		t.Fatalf("seed: %+v %v", r, err)
	}
	id := seedEntity(t, s, ctx, "starter/notes/start")
	variables := seedEntity(t, s, ctx, "starter/notes/variables")
	if id == 1 {
		t.Fatal("expected global IDs to be remapped")
	}
	var text string
	if err := s.DB.QueryRowContext(ctx, `SELECT description FROM note WHERE id=700`).Scan(&text); err != nil || text != "note=700 script=999" {
		t.Fatalf("existing changed: %q %v", text, err)
	}
	r, err = s.SeedStarter(ctx)
	if err != nil || r.Added != 0 || r.Updated != 0 {
		t.Fatalf("repeat: %+v %v", r, err)
	}
	// Renaming an example protects it; an untouched sibling upgrades in place.
	if _, err := s.DB.ExecContext(ctx, `UPDATE note SET title='my renamed guide' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	var set starterSet
	_ = json.Unmarshal(starterJSON, &set)
	set.Version = 2
	set.Tables["note"][0]["title"] = "New start"
	set.Tables["note"][1]["title"] = "New variables"
	data, _ := json.Marshal(set)
	r, err = s.seedStarter(ctx, data)
	if err != nil || r.Updated != 1 {
		t.Fatalf("update: %+v %v", r, err)
	}
	var title string
	_ = s.DB.QueryRowContext(ctx, `SELECT title FROM note WHERE id=?`, id).Scan(&title)
	if title != "my renamed guide" {
		t.Fatal(title)
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT title FROM note WHERE id=?`, variables).Scan(&title)
	if title != "New variables" {
		t.Fatal(title)
	}
	// Purging leaves seed identity behind; it must not resurrect on the next version.
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM note WHERE id=?`, variables); err != nil {
		t.Fatal(err)
	}
	set.Version = 3
	data, _ = json.Marshal(set)
	if _, err = s.seedStarter(ctx, data); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM note WHERE id=?`, variables).Scan(&n)
	if n != 0 {
		t.Fatal("resurrected")
	}
}
func TestStarterReferencesGraphsAndRemoval(t *testing.T) {
	s, ctx := starterStore(t)
	if _, err := s.SeedStarter(ctx); err != nil {
		t.Fatal(err)
	}
	script := seedEntity(t, s, ctx, "starter/scripts/dice")
	note := seedEntity(t, s, ctx, "starter/notes/variables")
	var text string
	_ = s.DB.QueryRowContext(ctx, `SELECT description FROM note WHERE id=?`, note).Scan(&text)
	if !strings.Contains(text, "script="+itoa(script)+"{2к6}") {
		t.Fatal("script reference not remapped", text)
	}
	for _, key := range []string{"dice", "progress", "freshness", "emit", "hook"} {
		id := seedEntity(t, s, ctx, "starter/scripts/"+key)
		script, err := s.Execution.GetScript(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		g, err := graph.Decode([]byte(script.Graph))
		if err != nil {
			t.Fatal(err)
		}
		if problems := g.Validate(graph.Builtin()); len(problems) > 0 {
			t.Fatalf("%s: %+v", key, problems)
		}
	}
	n, err := s.RemoveStarter(ctx)
	if err != nil || n == 0 {
		t.Fatalf("remove: %d %v", n, err)
	}
	r, err := s.SeedStarter(ctx)
	if err != nil || r.Added != 0 || r.Updated != 0 {
		t.Fatalf("after removal: %+v %v", r, err)
	}
}

func TestStarterRemovalKeepsEditsAndTheirOwners(t *testing.T) {
	s, ctx := starterStore(t)
	if _, err := s.SeedStarter(ctx); err != nil {
		t.Fatal(err)
	}
	child := seedEntity(t, s, ctx, "starter/notes/variables")
	quest := seedEntity(t, s, ctx, "starter/quests/first")
	step := seedEntity(t, s, ctx, "starter/steps/first-1")
	if _, err := s.DB.ExecContext(ctx, `UPDATE note SET pinned=1 WHERE id=?`, child); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE queststep SET progress_current=1 WHERE id=?`, step); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveStarter(ctx); err != nil {
		t.Fatal(err)
	}
	for table, id := range map[string]int64{"note": child, "quest": quest} {
		var n int
		if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE id=?", id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("edited %s lost: %d %v", table, n, err)
		}
	}
}
func TestStarterScriptUpgradePreservesOldVersions(t *testing.T) {
	s, ctx := starterStore(t)
	if _, err := s.SeedStarter(ctx); err != nil {
		t.Fatal(err)
	}
	id := seedEntity(t, s, ctx, "starter/scripts/dice")
	var old string
	_ = s.DB.QueryRowContext(ctx, `SELECT graph FROM script_version WHERE script_id=? AND version=1`, id).Scan(&old)
	var set starterSet
	_ = json.Unmarshal(starterJSON, &set)
	set.Version = 2
	set.Tables["script"][0]["description"] = "new description"
	data, _ := json.Marshal(set)
	if _, err := s.seedStarter(ctx, data); err != nil {
		t.Fatal(err)
	}
	var version int
	_ = s.DB.QueryRowContext(ctx, `SELECT version FROM script WHERE id=?`, id).Scan(&version)
	if version != 2 {
		t.Fatal(version)
	}
	var previous string
	_ = s.DB.QueryRowContext(ctx, `SELECT graph FROM script_version WHERE script_id=? AND version=1`, id).Scan(&previous)
	if previous != old {
		t.Fatal("old version changed")
	}
}

func TestStarterRemovalPreservesTaggedQuest(t *testing.T) {
	s, ctx := starterStore(t)
	if _, err := s.SeedStarter(ctx); err != nil {
		t.Fatal(err)
	}
	q := seedEntity(t, s, ctx, "starter/quests/first")
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO tag(slug,label,color,created_at) VALUES ('mine','mine','#fff',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO quest_tag(quest_id,tag_id) SELECT ?,id FROM tag WHERE slug='mine'`, q); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveStarter(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM quest WHERE id=?`, q).Scan(&n); err != nil || n != 1 {
		t.Fatalf("tagged quest removed: %d %v", n, err)
	}
}
