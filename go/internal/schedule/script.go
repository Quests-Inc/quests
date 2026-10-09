package schedule

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// commandCmd prepares a user-written command for execution. A plain command (one line or many) runs
// under `sh -c`; a command that starts with a shebang is a script with its own interpreter: it is
// written to a private temp file and executed as is. The returned cleanup removes that file and must
// be called after the command has finished.
func commandCmd(ctx context.Context, command string) (*exec.Cmd, func(), error) {
	if !strings.HasPrefix(strings.TrimLeft(command, " \t\r\n"), "#!") {
		return exec.CommandContext(ctx, "sh", "-c", command), func() {}, nil
	}
	f, err := os.CreateTemp("", "quests-script-*")
	if err != nil {
		return nil, func() {}, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	_, werr := f.WriteString(strings.TrimLeft(command, " \t\r\n"))
	cerr := f.Close()
	if werr != nil || cerr != nil {
		cleanup()
		if werr != nil {
			return nil, func() {}, werr
		}
		return nil, func() {}, cerr
	}
	if err := os.Chmod(path, 0o700); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return exec.CommandContext(ctx, path), cleanup, nil
}
