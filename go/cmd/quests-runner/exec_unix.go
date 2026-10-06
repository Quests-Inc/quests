//go:build unix

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// killTree makes a timed-out command die with everything it spawned: `sh -c`
// forks, and a child left holding stdout would keep Run() waiting.
func killTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
}
