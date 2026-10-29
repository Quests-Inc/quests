package archtest

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// execAllowed lists every non-test file that may start a process (import os/exec), and why. The server must run no
// user-written command (docs/automation.md): user code goes to a runner. A new entry here is a decision to review —
// say in the comment which of the rules still holds.
var execAllowed = map[string]string{
	"internal/platform/filepreview/preview.go": "fixed pdftotext executable and flags, bounded attachment bytes on stdin; no user command, shell, or filename; request deadline limits execution",
	"cmd/quests-runner/exec.go":                "the runner: the one place user commands run, on the owner's device",
	"cmd/quests-runner/exec_other.go":          "the runner (non-unix process-group handling)",
	"cmd/quests-runner/exec_unix.go":           "the runner (unix process-group handling)",
	"internal/schedule/backup_pg.go":           "pg_dump of the server's own database: a fixed command, no user input",
	"internal/overlay/shell/monitors.go":       "the HUD on a workstation (window-manager calls), not part of the server",
	"internal/mcp/tools_templates.go":          "dry_run_emit_pool: runs on the MCP server's machine (the user's own, over stdio), never on the API host",
}

func TestOnlyAllowedPackagesStartProcesses(t *testing.T) {
	root := filepath.Join("..", "..")
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(b), `"os/exec"`) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		if _, ok := execAllowed[rel]; !ok {
			t.Errorf("%s imports os/exec: the server runs no commands — user code goes to a runner (docs/automation.md). "+
				"If this is a fixed, non-user command, add the file to execAllowed with the reason.", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// An allowance nobody uses any more is dead weight that would let a process start again unnoticed.
	var stale []string
	for rel := range execAllowed {
		if !seen[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	for _, rel := range stale {
		t.Errorf("%s is in execAllowed but no longer imports os/exec: remove the entry", rel)
	}
}
