package schedule

import (
	"os"
	"strings"
)

// secretEnv are process variables that must never reach a command run from a
// check or an emit pool: database credentials of the server itself. Everything
// else (the root .env the operator deliberately shares with their scripts)
// passes through, as documented for emit_pool_command.
var secretEnv = map[string]bool{
	"QUESTS_DATABASE_URL": true,
	"PGPASSWORD":          true,
	"PGPASSFILE":          true,
	"PGSERVICEFILE":       true,
}

// CommandEnv is os.Environ() without the server's database credentials.
func CommandEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if secretEnv[name] || strings.HasPrefix(name, "POSTGRES_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
