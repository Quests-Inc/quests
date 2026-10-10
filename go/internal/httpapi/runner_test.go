package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/pgtest"
	"github.com/valirum/quests/go/internal/store"
)

// rr sends a JSON request with an optional bearer token and decodes a JSON object answer.
func rr(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func newRunnerEnv(t *testing.T, label string) (*Server, http.Handler, string) {
	t.Helper()
	dav := newFakeDAV()
	t.Cleanup(dav.Close)
	s := newAttachmentServer(t, dav.URL, fakeClamd(t, "stream: OK"), 0)
	h := s.Handler()
	code, out := rr(t, h, "POST", "/api/runners", "", map[string]any{"label": label})
	if code != 201 {
		t.Fatalf("create runner = %d %v", code, out)
	}
	code, out = rr(t, h, "POST", "/api/runner/register", "", map[string]any{"registration_token": out["registration_token"], "hostname": "pc", "version": "t"})
	if code != 200 {
		t.Fatalf("register = %d %v", code, out)
	}
	return s, h, out["token"].(string)
}

func TestRunnerJobLifecycle(t *testing.T) {
	s, h, token := newRunnerEnv(t, "home")
	ctx := t.Context()

	// A second registration with the same token must not work.
	code, _ := rr(t, h, "POST", "/api/runner/register", "", map[string]any{"registration_token": "nope"})
	if code != 401 {
		t.Fatalf("bad registration token = %d", code)
	}
	if code, _ := rr(t, h, "GET", "/api/runner/jobs?wait=0", "", nil); code != 401 {
		t.Fatalf("poll without token = %d", code)
	}

	// Nothing queued: 204.
	if code, _ := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil); code != 204 {
		t.Fatalf("empty poll = %d", code)
	}

	job, created, err := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindOnce, Command: "echo hi", DedupeKey: "step:1:p", Title: "шаг"})
	if err != nil || !created {
		t.Fatalf("enqueue: %v %v", created, err)
	}
	// Same key while unfinished: the same job, not a new one.
	again, created2, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindOnce, Command: "echo hi", DedupeKey: "step:1:p"})
	if created2 || again.ID != job.ID {
		t.Fatalf("dedupe: created=%v id %d vs %d", created2, again.ID, job.ID)
	}

	code, got := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil)
	if code != 200 || got["command"] != "echo hi" {
		t.Fatalf("claim = %d %v", code, got)
	}
	lease := got["lease_id"].(string)
	id := int64(got["id"].(float64))
	// Taken: nobody else gets it.
	if code, _ := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil); code != 204 {
		t.Fatalf("second claim = %d", code)
	}

	hb := fmt.Sprintf("/api/runner/jobs/%d/heartbeat", id)
	if code, _ := rr(t, h, "POST", hb, token, map[string]any{"lease_id": "wrong"}); code != 409 {
		t.Fatalf("heartbeat with a wrong lease = %d", code)
	}
	code, out := rr(t, h, "POST", hb, token, map[string]any{"lease_id": lease, "progress_current": 3, "progress_total": 10, "log_tail": "working"})
	if code != 200 || out["cancel"] != false {
		t.Fatalf("heartbeat = %d %v", code, out)
	}
	cur, _ := s.Store.GetRunnerJob(ctx, id)
	if cur.State != store.JobRunning || cur.ProgressCurrent == nil || *cur.ProgressCurrent != 3 || cur.LogTail != "working" {
		t.Fatalf("after heartbeat: %+v", cur)
	}

	var done []store.RunnerJob
	s.RunnerDone = func(_ context.Context, j store.RunnerJob) { done = append(done, j) }
	cp := fmt.Sprintf("/api/runner/jobs/%d/complete", id)
	code, out = rr(t, h, "POST", cp, token, map[string]any{"lease_id": lease, "exit": 0, "stdout": "42\n"})
	if code != 200 || out["accepted"] != true {
		t.Fatalf("complete = %d %v", code, out)
	}
	// The same delivery again changes nothing and does not call back twice.
	code, out = rr(t, h, "POST", cp, token, map[string]any{"lease_id": lease, "exit": 0, "stdout": "42\n"})
	if code != 200 || out["accepted"] != false {
		t.Fatalf("repeat complete = %d %v", code, out)
	}
	if len(done) != 1 || done[0].State != store.JobSuccess {
		t.Fatalf("RunnerDone calls: %+v", done)
	}
	// Finished: a new job with the same key may be queued (attempt 2).
	next, created3, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindOnce, Command: "echo hi", DedupeKey: "step:1:p"})
	if !created3 || next.AttemptNo != 2 {
		t.Fatalf("second attempt: created=%v attempt=%d", created3, next.AttemptNo)
	}
}

func TestRunnerExpiryAndLateResult(t *testing.T) {
	s, h, token := newRunnerEnv(t, "home")
	ctx := t.Context()
	j, _, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindOnce, Command: "sleep 1", DedupeKey: "k1"})
	_, got := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil)
	lease := got["lease_id"].(string)

	// The runner goes silent: the lease lapses → lost.
	past := time.Now().UTC().Add(-time.Minute).Format("2006-01-02 15:04:05.000000")
	if _, err := s.Store.DB.Exec(`UPDATE runner_job SET lease_until = ? WHERE id = ?`, past, j.ID); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Store.ExpireJobs(ctx)
	if err != nil || len(changed) != 1 || changed[0].State != store.JobLost {
		t.Fatalf("expire: %v %+v", err, changed)
	}
	// It comes back online with the result it finished meanwhile: accepted while still valid.
	code, out := rr(t, h, "POST", fmt.Sprintf("/api/runner/jobs/%d/complete", j.ID), token, map[string]any{"lease_id": lease, "exit": 0})
	if code != 200 || out["accepted"] != true {
		t.Fatalf("late complete = %d %v", code, out)
	}
	if cur, _ := s.Store.GetRunnerJob(ctx, j.ID); cur.State != store.JobSuccess {
		t.Fatalf("state after late result: %s", cur.State)
	}

	// A job nobody picked up in time is skipped, not failed.
	q, _, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "away", Kind: store.JobKindOnce, Command: "true", DedupeKey: "k2"})
	if _, err := s.Store.DB.Exec(`UPDATE runner_job SET valid_until = ? WHERE id = ?`, past, q.ID); err != nil {
		t.Fatal(err)
	}
	changed, _ = s.Store.ExpireJobs(ctx)
	if len(changed) != 1 || changed[0].State != store.JobSkipped {
		t.Fatalf("overdue queued: %+v", changed)
	}
}

func TestRunnerCancelRevokeAndOrder(t *testing.T) {
	s, h, token := newRunnerEnv(t, "home")
	ctx := t.Context()
	poll, _, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindPoll, Command: "echo 1", DedupeKey: "p"})
	once, _, _ := s.Store.EnqueueJob(ctx, store.JobSpec{RunnerLabel: "home", Kind: store.JobKindOnce, Command: "echo 2", DedupeKey: "o"})
	// A one-shot goes before an older poll.
	_, got := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil)
	if int64(got["id"].(float64)) != once.ID {
		t.Fatalf("order: got job %v, want the one-shot %d", got["id"], once.ID)
	}
	lease := got["lease_id"].(string)

	// Cancelling a running job is heard in the next heartbeat.
	if err := s.Store.CancelRunnerJob(ctx, once.ID); err != nil {
		t.Fatal(err)
	}
	_, out := rr(t, h, "POST", fmt.Sprintf("/api/runner/jobs/%d/heartbeat", once.ID), token, map[string]any{"lease_id": lease})
	if out["cancel"] != true {
		t.Fatalf("cancel not delivered: %v", out)
	}
	// A waiting job is dropped at once.
	if err := s.Store.CancelRunnerJob(ctx, poll.ID); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Store.GetRunnerJob(ctx, poll.ID); cur.State != store.JobCanceled {
		t.Fatalf("waiting job after cancel: %s", cur.State)
	}

	// Revoking the runner closes its token.
	if code, _ := rr(t, h, "DELETE", "/api/runners/home", "", nil); code != 204 {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil); code != 401 {
		t.Fatalf("revoked runner polls = %d", code)
	}
}

// A runner is the only place a command executes, so the server is also the only place the secrets of
// what the job belongs to reach it from: they ride in the job's environment, and whatever the runner
// prints back is masked before it is stored.
func TestRunnerReceivesOwnersSecretsAndOutputIsMasked(t *testing.T) {
	s, h, token := newRunnerEnv(t, "home")
	ctx := t.Context()

	tmplID := pgtest.SeedTemplate(t, s.Store.DB, "почта")
	if err := s.Store.SetTemplateSecret(ctx, tmplID, "MAIL_PASSWORD", "s3cr3t-pw"); err != nil {
		t.Fatal(err)
	}
	pgtest.Seed(t, s.Store.DB, `INSERT INTO note (id, title, created_at, updated_at) VALUES (77, 'заметка', now(), now())`)
	if err := s.Store.SetSecret(ctx, store.SecretOwner{Kind: store.SecretOwnerNote, ID: 77}, "NOTE_TOKEN", "note-secret"); err != nil {
		t.Fatal(err)
	}
	noteID := int64(77)
	for _, spec := range []store.JobSpec{
		{RunnerLabel: "home", Kind: store.JobKindEmitPool, TemplateID: &tmplID, Command: "echo $MAIL_PASSWORD", DedupeKey: "emit:t"},
		{RunnerLabel: "home", Kind: store.JobKindPoll, NoteID: &noteID, Command: "echo $NOTE_TOKEN", DedupeKey: "note:77:poll"},
	} {
		if _, _, err := s.Store.EnqueueJob(ctx, spec); err != nil {
			t.Fatal(err)
		}
	}

	want := map[string]string{"MAIL_PASSWORD": "s3cr3t-pw", "NOTE_TOKEN": "note-secret"}
	for i := 0; i < 2; i++ {
		code, got := rr(t, h, "GET", "/api/runner/jobs?wait=0", token, nil)
		if code != 200 {
			t.Fatalf("claim %d = %d %v", i, code, got)
		}
		env, _ := got["env"].(map[string]any)
		var name, value string
		for k, v := range want {
			if x, ok := env[k]; ok {
				name, value = k, v
				if x != v {
					t.Fatalf("%s = %v, want the secret", k, x)
				}
			}
		}
		if name == "" {
			t.Fatalf("job %v carries none of the owner's secrets: %v", got["id"], env)
		}
		delete(want, name)

		id := int64(got["id"].(float64))
		cp := fmt.Sprintf("/api/runner/jobs/%d/complete", id)
		if code, out := rr(t, h, "POST", cp, token, map[string]any{"lease_id": got["lease_id"], "exit": 1, "stdout": "got " + value, "stderr": "bad " + value}); code != 200 {
			t.Fatalf("complete = %d %v", code, out)
		}
		stored, _ := s.Store.GetRunnerJob(ctx, id)
		if strings.Contains(stored.Stdout+stored.Stderr, value) || !strings.Contains(stored.Stdout, "***") {
			t.Fatalf("the secret reached the stored output: %q / %q", stored.Stdout, stored.Stderr)
		}
	}
	if len(want) != 0 {
		t.Fatalf("secrets never delivered: %v", want)
	}
}
