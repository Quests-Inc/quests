package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer plays the server side of the runner protocol.
type fakeServer struct {
	mu           sync.Mutex
	jobs         []job
	completed    []result
	beats        []heartbeatIn
	cancelBeats  bool
	failComplete int // number of complete calls to answer with 500 first
}

func (f *fakeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/runner/jobs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.jobs) == 0 {
			w.WriteHeader(204)
			return
		}
		j := f.jobs[0]
		f.jobs = f.jobs[1:]
		_ = json.NewEncoder(w).Encode(j)
	})
	mux.HandleFunc("POST /api/runner/jobs/{id}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var in heartbeatIn
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.beats = append(f.beats, in)
		cancel := f.cancelBeats
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"cancel": cancel})
	})
	mux.HandleFunc("POST /api/runner/jobs/{id}/complete", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failComplete > 0 {
			f.failComplete--
			http.Error(w, "boom", 500)
			return
		}
		var in struct {
			LeaseID  string `json:"lease_id"`
			Exit     int    `json:"exit"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			TimedOut bool   `json:"timed_out"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.completed = append(f.completed, result{LeaseID: in.LeaseID, Exit: in.Exit, Stdout: in.Stdout, Stderr: in.Stderr, TimedOut: in.TimedOut})
		_, _ = w.Write([]byte(`{"ok":true,"accepted":true}`))
	})
	return mux
}

func newTestAgent(t *testing.T, f *fakeServer) *agent {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	a := newAgent(config{Server: srv.URL, Token: "t", Label: "home"}, newClient(srv.URL, "t"), t.TempDir())
	a.pollWait = time.Second
	a.hbEvery = 150 * time.Millisecond
	return a
}

func (f *fakeServer) waitCompleted(t *testing.T, n int) []result {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := append([]result(nil), f.completed...)
		f.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("expected %d delivered results", n)
	return nil
}

func TestAgentRunsJobReportsProgressAndEnv(t *testing.T) {
	f := &fakeServer{jobs: []job{{ID: 1, LeaseID: "L1", Kind: "once", TimeoutSeconds: 10,
		Command: `echo "secret=$MY_SECRET job=$QUESTS_JOB_ID"; printf 'progress=2 total=4\n'; sleep 0.5; echo done`,
		Env:     map[string]string{"MY_SECRET": "xyz"}}}}
	a := newTestAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.run(ctx, 1) }()

	res := f.waitCompleted(t, 1)[0]
	if res.LeaseID != "L1" || res.Exit != 0 || !strings.Contains(res.Stdout, "secret=xyz job=1") || !strings.Contains(res.Stdout, "done") {
		t.Fatalf("result: %+v", res)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	sawProgress := false
	for _, b := range f.beats {
		if b.ProgressCurrent != nil && *b.ProgressCurrent == 2 && *b.ProgressTotal == 4 && strings.Contains(b.LogTail, "secret=") {
			sawProgress = true
		}
	}
	if !sawProgress {
		t.Fatalf("no heartbeat carried the progress and the log tail: %+v", f.beats)
	}
}

func TestAgentTimeoutKillsTheWholeTree(t *testing.T) {
	f := &fakeServer{jobs: []job{{ID: 2, LeaseID: "L2", Kind: "poll", TimeoutSeconds: 1, Command: `sleep 30; echo late`}}}
	a := newTestAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	go func() { _ = a.run(ctx, 1) }()
	res := f.waitCompleted(t, 1)[0]
	if !res.TimedOut || res.Exit == 0 {
		t.Fatalf("result: %+v", res)
	}
	if time.Since(start) > 6*time.Second {
		t.Fatalf("took %s: the child was not killed", time.Since(start))
	}
}

func TestAgentStopsOnServerCancel(t *testing.T) {
	f := &fakeServer{cancelBeats: true, jobs: []job{{ID: 3, LeaseID: "L3", Kind: "once", TimeoutSeconds: 60, Command: `sleep 30`}}}
	a := newTestAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.run(ctx, 1) }()
	res := f.waitCompleted(t, 1)[0]
	if res.TimedOut || res.Exit == 0 || !strings.Contains(res.Stderr, "отменено сервером") {
		t.Fatalf("result: %+v", res)
	}
}

func TestAgentKeepsUndeliveredResultAndSendsItLater(t *testing.T) {
	f := &fakeServer{failComplete: 1, jobs: []job{{ID: 4, LeaseID: "L4", Kind: "once", TimeoutSeconds: 10, Command: `echo hi`}}}
	a := newTestAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.run(ctx, 1) }()

	// The first delivery fails (500), the next poll round flushes the spool.
	res := f.waitCompleted(t, 1)[0]
	if res.LeaseID != "L4" || !strings.Contains(res.Stdout, "hi") {
		t.Fatalf("result: %+v", res)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if entries, _ := os.ReadDir(a.spool); len(entries) == 0 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("spool file was not removed after delivery")
}

func TestParseProgress(t *testing.T) {
	if c, tot, ok := parseProgress("a\nprogress=1 total=5\nb\nprogress=3 total=5\n"); !ok || c != 3 || tot != 5 {
		t.Fatalf("got %d %d %v", c, tot, ok)
	}
	if _, _, ok := parseProgress("progress=3\n"); ok {
		t.Fatal("a line without total must not count")
	}
}

func TestAgentRunsScriptsWithAShebang(t *testing.T) {
	f := &fakeServer{jobs: []job{{ID: 5, LeaseID: "L5", Kind: "emit_pool", TimeoutSeconds: 10,
		Command: "#!/bin/sh\necho from-script-$QUESTS_JOB_ID\n"}}}
	a := newTestAgent(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.run(ctx, 1) }()
	res := f.waitCompleted(t, 1)[0]
	if res.Exit != 0 || !strings.Contains(res.Stdout, "from-script-5") {
		t.Fatalf("result: %+v", res)
	}
}
