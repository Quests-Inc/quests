package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type agent struct {
	cfg   config
	cl    *client
	spool string
	// pollWait is one long-poll; hbEvery how often a running job renews its lease.
	pollWait time.Duration
	hbEvery  time.Duration

	flushMu sync.Mutex
}

func newAgent(cfg config, cl *client, spool string) *agent {
	return &agent{cfg: cfg, cl: cl, spool: spool, pollWait: 25 * time.Second, hbEvery: 20 * time.Second}
}

// run polls with n workers until ctx ends. It only returns early when the
// server stops accepting the token.
func (a *agent) run(ctx context.Context, n int) error {
	errc := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.worker(ctx); err != nil {
				errc <- err
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case err := <-errc:
		return err
	case <-done:
		return nil
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (a *agent) worker(ctx context.Context) error {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		a.flushSpool(ctx)
		j, err := a.cl.poll(ctx, a.pollWait)
		if err != nil {
			if errors.Is(err, errUnauthorized) {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("poll: %v (retry in %s)", err, backoff)
			sleepCtx(ctx, backoff)
			if backoff *= 2; backoff > time.Minute {
				backoff = time.Minute
			}
			continue
		}
		backoff = 2 * time.Second
		if j != nil {
			a.execute(ctx, j)
		}
	}
	return nil
}

// execute runs one job: heartbeats while it works, then delivers the result
// (or keeps it in the spool when the server cannot be reached).
func (a *agent) execute(ctx context.Context, j *job) {
	log.Printf("job %d (%s, attempt %d): %s", j.ID, j.Kind, j.AttemptNo, j.Title)
	timeout := time.Duration(j.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = time.Minute
	}
	env := map[string]string{}
	for k, v := range j.Env {
		env[k] = v
	}
	env["QUESTS_JOB_ID"] = fmt.Sprint(j.ID)
	env["QUESTS_JOB_KIND"] = j.Kind
	if j.StepID != nil {
		env["QUESTS_STEP_ID"] = fmt.Sprint(*j.StepID)
	}
	if j.NoteID != nil {
		env["QUESTS_NOTE_ID"] = fmt.Sprint(*j.NoteID)
	}
	if a.cfg.Server != "" {
		env["QUESTS_API"] = a.cfg.Server
	}

	var stdout, stderr = &tailBuf{cap: outputKeep}, &tailBuf{cap: outputKeep}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	var serverCanceled bool
	var cmu sync.Mutex

	hbDone := make(chan struct{})
	hbStop := make(chan struct{})
	go func() {
		defer close(hbDone)
		beat := func() {
			in := heartbeatIn{LeaseID: j.LeaseID, LogTail: stdout.last(tailKeep)}
			if in.LogTail == "" {
				in.LogTail = stderr.last(tailKeep)
			}
			if cur, total, ok := parseProgress(stdout.String()); ok {
				in.ProgressCurrent, in.ProgressTotal = &cur, &total
			}
			hctx, c := context.WithTimeout(context.Background(), 15*time.Second)
			defer c()
			cancel, err := a.cl.heartbeat(hctx, j.ID, in)
			switch {
			case err == nil && cancel:
				cmu.Lock()
				serverCanceled = true
				cmu.Unlock()
				cancelRun()
			case errors.Is(err, errLeaseLost):
				// The server gave up on this lease; finish anyway, a late result may still be taken.
			case err != nil:
				log.Printf("job %d heartbeat: %v", j.ID, err)
			}
		}
		beat()
		t := time.NewTicker(a.hbEvery)
		defer t.Stop()
		for {
			select {
			case <-hbStop:
				return
			case <-t.C:
				beat()
			}
		}
	}()

	exit, timedOut, canceled := runCommand(runCtx, j.Command, env, timeout, stdout, stderr)
	close(hbStop)
	<-hbDone

	errOut := stderr.String()
	cmu.Lock()
	byServer := serverCanceled
	cmu.Unlock()
	switch {
	case byServer:
		errOut += "\nотменено сервером"
	case canceled:
		errOut += "\nраннер остановлен"
	}
	res := result{JobID: j.ID, LeaseID: j.LeaseID, Exit: exit, Stdout: stdout.String(), Stderr: errOut, TimedOut: timedOut}
	a.deliver(res)
}

func (a *agent) deliver(res result) {
	dctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	err := a.cl.complete(dctx, res)
	switch {
	case err == nil:
		log.Printf("job %d delivered (exit %d)", res.JobID, res.Exit)
	case errors.Is(err, errLeaseLost), errors.Is(err, errUnauthorized):
		log.Printf("job %d: server does not take the result (%v); dropped", res.JobID, err)
	default:
		log.Printf("job %d: cannot deliver (%v); kept to retry", res.JobID, err)
		if serr := a.saveSpool(res); serr != nil {
			log.Printf("job %d: spool: %v", res.JobID, serr)
		}
	}
}

func (a *agent) saveSpool(res result) error {
	if err := os.MkdirAll(a.spool, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(res)
	return os.WriteFile(filepath.Join(a.spool, fmt.Sprintf("%d.json", res.JobID)), b, 0o600)
}

// flushSpool tries to deliver results kept from earlier. A refused result
// (lease lost / token) is dropped, a network failure stops the round.
func (a *agent) flushSpool(ctx context.Context) {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()
	entries, err := os.ReadDir(a.spool)
	if err != nil {
		return
	}
	for _, e := range entries {
		if ctx.Err() != nil || e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(a.spool, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var res result
		if json.Unmarshal(b, &res) != nil || res.JobID == 0 {
			_ = os.Remove(path)
			continue
		}
		dctx, c := context.WithTimeout(ctx, 30*time.Second)
		err = a.cl.complete(dctx, res)
		c()
		switch {
		case err == nil, errors.Is(err, errLeaseLost):
			_ = os.Remove(path)
			log.Printf("job %d: kept result delivered", res.JobID)
		case errors.Is(err, errUnauthorized):
			return
		default:
			return
		}
	}
}
