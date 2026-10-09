package schedule

import (
	"context"
	"strings"
	"testing"
)

func runScript(t *testing.T, command string) string {
	t.Helper()
	cmd, cleanup, err := commandCmd(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestCommandCmdMultilineAndShebang(t *testing.T) {
	if got := runScript(t, "A=1\nB=2\necho $((A+B))"); got != "3" {
		t.Fatalf("multi-line sh = %q", got)
	}
	if got := runScript(t, "\n  #!/bin/sh\necho \"$0\" | grep -c quests-script"); got != "1" {
		t.Fatalf("a script with a shebang must run as its own file, got %q", got)
	}
	if got := runScript(t, "#!/usr/bin/env python3\nprint(6*7)"); got != "42" {
		t.Fatalf("python script = %q", got)
	}
}
