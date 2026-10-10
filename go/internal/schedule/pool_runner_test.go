package schedule

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/domain"
)

func withInlineWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := emitPoolInlineWait
	emitPoolInlineWait = d
	t.Cleanup(func() { emitPoolInlineWait = old })
}

// A fast command still finishes inside the same tick (inline wait), so the
// background runner doesn't add a tick of latency for ordinary scripts.
func TestPoolRunnerFastCommandReadyInSameCall(t *testing.T) {
	withInlineWait(t, 2*time.Second)
	st := openEmitPoolDB(t)
	r := newPoolRunner()
	tmpl := poolTemplate(30, `echo '{"steps":[{"title":"a"}]}'`, 1)
	items, _, _, err := resolveEmitPoolOpts(context.Background(), st, tmpl, "p", time.Now(), rand.New(rand.NewSource(1)), poolOpts{runner: r})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("fast command should be collected in the same call, got %+v", items)
	}
}

// A slow command must not hold the caller: the call returns immediately with
// nothing, no second process is started while one runs, and once it is done
// a later call collects the result exactly once.
func TestPoolRunnerSlowCommandDoesNotBlockAndRunsOnce(t *testing.T) {
	withInlineWait(t, 20*time.Millisecond)
	st := openEmitPoolDB(t)
	r := newPoolRunner()
	counter := filepath.Join(t.TempDir(), "runs")
	cmd := "#!/bin/sh\necho x >> " + counter + "\nsleep 1\necho '{\"steps\":[{\"title\":\"slow\"}]}'\n"
	tmpl := poolTemplate(31, cmd, 1)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(1))

	start := time.Now()
	for i := 0; i < 5; i++ {
		items, _, _, err := resolveEmitPoolOpts(ctx, st, tmpl, "p", time.Now(), rng, poolOpts{runner: r})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Fatalf("call %d: result cannot be ready yet, got %+v", i, items)
		}
	}
	if elapsed := time.Since(start); elapsed > 700*time.Millisecond {
		t.Fatalf("5 ticks took %s: the slow command blocked the caller", elapsed)
	}

	var got []domain.Step
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(got) == 0 {
		time.Sleep(100 * time.Millisecond)
		items, _, _, err := resolveEmitPoolOpts(ctx, st, tmpl, "p", time.Now(), rng, poolOpts{runner: r})
		if err != nil {
			t.Fatal(err)
		}
		got = items
	}
	if len(got) != 1 || got[0].Title != "slow" {
		t.Fatalf("result never collected: %+v", got)
	}
	b, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Fatalf("command ran %d times, want exactly 1", n)
	}
}

// Editing the command or rolling over to a new period must not hand out a
// stale result.
func TestPoolRunnerKeyIncludesPeriodAndCommand(t *testing.T) {
	a := poolJobKey(1, "2026-10-01", "echo a")
	if a == poolJobKey(1, "2026-10-02", "echo a") || a == poolJobKey(1, "2026-10-01", "echo b") || a == poolJobKey(2, "2026-10-01", "echo a") {
		t.Fatal("job key must differ by period, command and template")
	}
}

// Hammer the runner from several goroutines (run with -race).
func TestPoolRunnerConcurrentCallers(t *testing.T) {
	withInlineWait(t, 10*time.Millisecond)
	st := openEmitPoolDB(t)
	r := newPoolRunner()
	done := make(chan struct{})
	for g := 0; g < 4; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			cmd := `echo '{"title":"c"}'`
			for i := 0; i < 20; i++ {
				_, _, _, _ = r.exec(context.Background(), st, int64(100+g), "p", cmd)
				time.Sleep(time.Millisecond)
			}
		}(g)
	}
	for g := 0; g < 4; g++ {
		<-done
	}
}
