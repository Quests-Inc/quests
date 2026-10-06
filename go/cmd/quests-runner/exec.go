package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tailBuf keeps the last cap bytes written to it: the end of the output is
// what a check's verdict is read from.
type tailBuf struct {
	mu  sync.Mutex
	cap int
	buf []byte
	cut bool
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.cap {
		t.buf = t.buf[len(t.buf)-t.cap:]
		t.cut = true
	}
	return len(p), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := string(t.buf)
	if t.cut {
		s = "…" + s
	}
	return s
}

func (t *tailBuf) last(n int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.buf
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(bytes.ToValidUTF8(b, nil))
}

const (
	outputKeep = 512 << 10 // what is sent back per stream
	tailKeep   = 2000      // log tail in a heartbeat
)

var progressLine = regexp.MustCompile(`(?m)^progress=(\d+) total=(\d+)\s*$`)

// parseProgress reads the last `progress=N total=M` line a command printed.
func parseProgress(out string) (cur, total int, ok bool) {
	all := progressLine.FindAllStringSubmatch(out, -1)
	if len(all) == 0 {
		return 0, 0, false
	}
	m := all[len(all)-1]
	cur, _ = strconv.Atoi(m[1])
	total, _ = strconv.Atoi(m[2])
	return cur, total, total >= 1
}

// runCommand runs `sh -c command` in the user's home with the job's env added,
// for at most timeout. canceled is set when ctx was cancelled from outside.
func runCommand(ctx context.Context, command string, env map[string]string, timeout time.Duration, stdout, stderr *tailBuf) (exit int, timedOut, canceled bool) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if strings.HasPrefix(strings.TrimLeft(command, " \t\r\n"), "#!") {
		// A script with its own interpreter line: written out and executed as is.
		f, err := os.CreateTemp("", "quests-runner-*")
		if err != nil {
			stderr.Write([]byte(err.Error() + "\n"))
			return -1, false, false
		}
		path := f.Name()
		defer os.Remove(path)
		_, werr := f.WriteString(command)
		cerr := f.Close()
		if werr != nil || cerr != nil || os.Chmod(path, 0o700) != nil {
			stderr.Write([]byte("cannot write the script\n"))
			return -1, false, false
		}
		cmd = exec.CommandContext(cctx, path)
	} else {
		cmd = exec.CommandContext(cctx, "sh", "-c", command)
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	killTree(cmd)
	err := cmd.Run()
	if err == nil {
		return 0, false, false
	}
	if cctx.Err() == context.DeadlineExceeded {
		return -1, true, false
	}
	if ctx.Err() != nil {
		return -1, false, true
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), false, false
	}
	stderr.Write([]byte(err.Error() + "\n"))
	return -1, false, false
}
