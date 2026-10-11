package config

import "testing"

func TestDatabaseURLFromEnv(t *testing.T) {
	clear := func() {
		for _, k := range []string{"QUESTS_DATABASE_URL", "QUESTS_DB_HOST", "QUESTS_DB_USER", "QUESTS_DB_PORT", "QUESTS_DB_NAME", "QUESTS_DB_SSLMODE", "POSTGRES_PASSWORD"} {
			t.Setenv(k, "")
		}
	}

	clear()
	if got := databaseURLFromEnv(); got != "" {
		t.Fatalf("nothing set: %q", got)
	}

	clear()
	t.Setenv("QUESTS_DATABASE_URL", " postgresql://u:p@h/db ")
	t.Setenv("QUESTS_DB_HOST", "ignored")
	if got := databaseURLFromEnv(); got != "postgresql://u:p@h/db" {
		t.Fatalf("an explicit URL wins: %q", got)
	}

	clear()
	t.Setenv("QUESTS_DB_HOST", "postgres")
	t.Setenv("POSTGRES_PASSWORD", "p@ss/w:rd#1")
	want := "postgresql://quests:p%40ss%2Fw%3Ard%231@postgres:5432/quests?sslmode=disable"
	if got := databaseURLFromEnv(); got != want {
		t.Fatalf("parts:\n got %q\nwant %q", got, want)
	}

	t.Setenv("QUESTS_DB_USER", "app")
	t.Setenv("QUESTS_DB_PORT", "6432")
	t.Setenv("QUESTS_DB_NAME", "main")
	t.Setenv("QUESTS_DB_SSLMODE", "require")
	if got, want := databaseURLFromEnv(), "postgresql://app:p%40ss%2Fw%3Ard%231@postgres:6432/main?sslmode=require"; got != want {
		t.Fatalf("overrides:\n got %q\nwant %q", got, want)
	}
}
