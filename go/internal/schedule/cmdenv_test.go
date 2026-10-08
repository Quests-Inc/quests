package schedule

import (
	"strings"
	"testing"
)

func TestCommandEnvDropsDatabaseCredentials(t *testing.T) {
	t.Setenv("QUESTS_DATABASE_URL", "postgresql://quests:secret@db/quests")
	t.Setenv("POSTGRES_PASSWORD", "secret")
	t.Setenv("PGPASSWORD", "secret")
	t.Setenv("SHARED_SCRIPT_KEY", "kept")
	var kept bool
	for _, kv := range CommandEnv() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "QUESTS_DATABASE_URL", "POSTGRES_PASSWORD", "PGPASSWORD":
			t.Fatalf("%s leaked into a command's environment", name)
		case "SHARED_SCRIPT_KEY":
			kept = true
		}
	}
	if !kept {
		t.Fatal("operator-shared variables must pass through")
	}
}
