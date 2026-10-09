package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSearchVariantsLayout(t *testing.T) {
	if got := SearchVariants("ghbdtn"); len(got) != 2 || got[1] != "привет" {
		t.Fatalf("latin keys: %v", got)
	}
	if got := SearchVariants("руддщ"); len(got) != 2 || got[1] != "hello" {
		t.Fatalf("cyrillic keys: %v", got)
	}
	if got := SearchVariants("deploy 2"); len(got) != 2 {
		t.Logf("mixed digits still switch layout: %v", got)
	}
	if got := SearchVariants("деплой deploy"); len(got) != 1 {
		t.Fatalf("a query in two scripts must stay as typed: %v", got)
	}
}

func TestPrefixTerm(t *testing.T) {
	for in, want := range map[string]string{"деплой": "деплой:*", "как депл": "депл:*", "ab": "", "деплой ": "", "деплой -борщ": "", `"архив вложений"`: ""} {
		if got := prefixTerm(in); got != want {
			t.Errorf("prefixTerm(%q) = %q, want %q", in, got, want)
		}
	}
}

const searchNotesSchema = `
CREATE TABLE note (
	id INTEGER PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	pinned INTEGER NOT NULL DEFAULT 0, is_readme INTEGER NOT NULL DEFAULT 0, is_private INTEGER NOT NULL DEFAULT 0,
	is_category INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, parent_id INTEGER,
	color TEXT, icon TEXT, custom_icon TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
INSERT INTO note (id, title, description, is_private, created_at, updated_at) VALUES
 (1, 'Деплой на прод', 'Как выкатить сервис: ci-deploy.sh, затем проверить здоровье.', 0, '2026-10-01 10:00:00', '2026-10-01 10:00:00'),
 (2, 'Мониторинг', 'Kuma, Prometheus, Grafana. Деплой не нужен.', 0, '2026-10-02 10:00:00', '2026-10-02 10:00:00'),
 (3, 'Пароли', 'деплой-секрет и прочее', 1, '2026-10-03 10:00:00', '2026-10-03 10:00:00'),
 (4, 'Рецепты', 'Борщ и пельмени', 0, '2026-10-04 10:00:00', '2026-10-04 10:00:00');
`

func TestSearchPlainRanksTitleAboveBodyAndHidesPrivate(t *testing.T) {
	db, err := sql.Open("sqlite", "file:search_"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(searchNotesSchema); err != nil {
		t.Fatal(err)
	}
	st := &Store{DB: db}
	hits, err := st.SearchNotes(context.Background(), "деплой", SearchOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].ID != 1 || hits[0].Where != "title" {
		t.Fatalf("hits = %+v (the title match must come first)", hits)
	}
	if !strings.Contains(hits[1].Snippet, MarkOpen+"Деплой"+MarkClose) && !strings.Contains(strings.ToLower(hits[1].Snippet), MarkOpen+"деплой"+MarkClose) {
		t.Fatalf("snippet does not mark the match: %q", hits[1].Snippet)
	}
	for _, h := range hits {
		if h.ID == 4 {
			t.Fatal("an unrelated note was found")
		}
	}
	hidden, _ := st.SearchNotes(context.Background(), "деплой", SearchOpts{ExcludePrivate: true})
	for _, h := range hidden {
		if h.ID == 3 {
			t.Fatal("a private note reached an agent's results")
		}
	}
	// Wrong layout: "ltgkjq" is "деплой" typed on a Latin keyboard.
	if h, _ := st.SearchNotes(context.Background(), "ltgkjq", SearchOpts{}); len(h) == 0 || h[0].ID != 1 {
		t.Fatalf("layout variant not searched: %+v", h)
	}
	if h, _ := st.SearchNotes(context.Background(), "   ", SearchOpts{}); len(h) != 0 {
		t.Fatal("blank query must return nothing")
	}
}
