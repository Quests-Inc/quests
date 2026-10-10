package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/valirum/quests/go/internal/pgtest"
)

func openTagTestDB(t *testing.T) *Store {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	return &Store{DB: pool, PGURL: url}
}

func TestNormalizeTagSlug(t *testing.T) {
	cases := map[string]string{
		"Frontend":  "frontend",
		"  API UI ": "api-ui",
		"MCP_tools": "mcp-tools",
		"фронт энд": "фронт-энд",
	}
	for in, want := range cases {
		if got := NormalizeTagSlug(in); got != want {
			t.Fatalf("%q → %q, want %q", in, got, want)
		}
	}
}

func TestCreateTagConflictReturnsExisting(t *testing.T) {
	st := openTagTestDB(t)
	ctx := context.Background()
	a, err := st.CreateTag(ctx, TagCreate{Slug: "frontend", Label: "Frontend"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateTag(ctx, TagCreate{Slug: "Frontend", Label: "Other"})
	if err != ErrConflict {
		t.Fatalf("err=%v want conflict", err)
	}
	if b.ID != a.ID {
		t.Fatalf("ids %d vs %d", b.ID, a.ID)
	}
}

func TestSetQuestTagsLimit(t *testing.T) {
	st := openTagTestDB(t)
	ctx := context.Background()
	qid := pgtest.SeedQuest(t, st.DB, "q")
	var ids []int64
	for i := 0; i < 6; i++ {
		tg, err := st.CreateTag(ctx, TagCreate{Slug: fmt.Sprintf("tag-%d", i), Label: "t"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tg.ID)
	}
	if err := st.SetQuestTags(ctx, qid, ids); err != ErrTooManyTags {
		t.Fatalf("err=%v want too many", err)
	}
	if err := st.SetQuestTags(ctx, qid, ids[:5]); err != nil {
		t.Fatal(err)
	}
	m, err := st.loadTagsForQuests(ctx, []int64{qid})
	if err != nil {
		t.Fatal(err)
	}
	if len(m[qid]) != 5 {
		t.Fatalf("got %d tags", len(m[qid]))
	}
}

func TestCopyTemplateTagsToQuest(t *testing.T) {
	st := openTagTestDB(t)
	ctx := context.Background()
	tmpl := pgtest.SeedTemplate(t, st.DB, "t")
	quest := pgtest.SeedQuest(t, st.DB, "q")
	tg, err := st.CreateTag(ctx, TagCreate{Slug: "api", Label: "API"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetTemplateTags(ctx, tmpl, []int64{tg.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.CopyTemplateTagsToQuest(ctx, tmpl, quest); err != nil {
		t.Fatal(err)
	}
	m, err := st.loadTagsForQuests(ctx, []int64{quest})
	if err != nil {
		t.Fatal(err)
	}
	if len(m[quest]) != 1 || m[quest][0].Slug != "api" {
		t.Fatalf("%v", m[quest])
	}
}
