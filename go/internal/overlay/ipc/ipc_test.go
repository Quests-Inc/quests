package ipc

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSocketPathEnv(t *testing.T) {
	t.Setenv("QUESTS_OVERLAY_SOCK", "/tmp/quests-test.sock")
	if SocketPath() != "/tmp/quests-test.sock" {
		t.Fatal(SocketPath())
	}
	t.Setenv("QUESTS_OVERLAY_SOCK", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	got := SocketPath()
	if filepath.Base(got) != "quests-overlay.sock" {
		t.Fatal(got)
	}
}

func TestStartSendStop(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "overlay.sock")
	t.Setenv("QUESTS_OVERLAY_SOCK", sock)

	srv, err := Start(func(cmd string) string {
		if cmd == "ping" {
			return "pong"
		}
		return "unknown:" + cmd
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	reply, err := Send("ping")
	if err != nil || reply != "pong" {
		t.Fatalf("reply=%q err=%v", reply, err)
	}
	reply, err = Send("status")
	if err != nil || reply != "unknown:status" {
		t.Fatalf("reply=%q err=%v", reply, err)
	}
}

func TestDispatchAndEmptyReply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUESTS_OVERLAY_SOCK", filepath.Join(dir, "d.sock"))

	srv, err := Start(func(string) string { return "" }, func(fn func()) { go fn() })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	reply, err := Send("x")
	if err != nil || reply != "ok" {
		t.Fatalf("reply=%q err=%v", reply, err)
	}
}

func TestSendMissingSocket(t *testing.T) {
	t.Setenv("QUESTS_OVERLAY_SOCK", filepath.Join(t.TempDir(), "nope.sock"))
	_, err := Send("x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInvokeTimeout(t *testing.T) {
	s := &Server{
		handler: func(string) string { time.Sleep(3 * time.Second); return "late" },
		dispatch: func(fn func()) {
			// never run fn → timeout
			_ = fn
		},
	}
	if got := s.invoke("x"); got != "error: timeout" {
		t.Fatalf("got %q", got)
	}
}

func TestInvokeNoHandler(t *testing.T) {
	s := &Server{}
	if got := s.invoke("x"); got != "error: no handler" {
		t.Fatalf("got %q", got)
	}
}
