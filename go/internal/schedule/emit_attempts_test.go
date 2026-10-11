package schedule

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/store"
)

func attemptsFor(t *testing.T, st *store.Store, templateID int64) []map[string]any {
	t.Helper()
	rows, err := st.Automation().ListEmitAttempts(bg(), templateID, 50)
	if err != nil {
		t.Fatalf("ListEmitAttempts: %v", err)
	}
	return rows
}

// Every execution leaves a row: the failures with their trace, the success
// with the step count.
func TestEmitAttemptsLogFailureThenSuccess(t *testing.T) {
	st := openTemplateSecretsDB(t)
	ctx := bg()
	fail := poolTemplate(5, `echo boom >&2; exit 3`, 1)
	now := time.Now()
	if _, _, _, err := resolveEmitPool(t, ctx, st, fail, "p1", now, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	ok := poolTemplate(5, `echo '{"steps":[{"title":"a"},{"title":"b"}]}'`, 1)
	later := now.Add(emitPoolRetryDelays[0] + time.Second) // past the pause
	if _, _, _, err := resolveEmitPool(t, ctx, st, ok, "p1", later, rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}

	rows := attemptsFor(t, st, 5)
	if len(rows) != 2 {
		t.Fatalf("want 2 attempts, got %d: %+v", len(rows), rows)
	}
	okRow, failRow := rows[0], rows[1] // newest first
	if failRow["status"] != "error" || failRow["attempt"].(int64) != 1 || !strings.Contains(failRow["message"].(string), "boom") {
		t.Errorf("bad failure row: %+v", failRow)
	}
	if okRow["status"] != "ok" || okRow["attempt"].(int64) != 2 || okRow["items"].(int64) != 2 || okRow["picked"].(int64) != 1 {
		t.Errorf("bad success row: %+v", okRow)
	}
}

func TestEmitAttemptsLogEmptyPoolMiss(t *testing.T) {
	st := openTemplateSecretsDB(t)
	if _, _, _, err := resolveEmitPool(t, bg(), st, poolTemplate(6, `echo null`, 0), "p", time.Now(), rand.New(rand.NewSource(1))); err != nil {
		t.Fatal(err)
	}
	rows := attemptsFor(t, st, 6)
	if len(rows) != 1 || rows[0]["status"] != "ok" || !strings.Contains(rows[0]["message"].(string), "miss: the command printed no quest") {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestEmitAttemptLogIsTrimmed(t *testing.T) {
	st := openTemplateSecretsDB(t)
	ctx := bg()
	tmpl := poolTemplate(12, `echo null`, 0)
	for i := 0; i < 205; i++ {
		// distinct period keys, each a fresh roll
		_, _, _, _ = resolveEmitPool(t, ctx, st, tmpl, "p"+time.Duration(i).String(), time.Now(), rand.New(rand.NewSource(1)))
	}
	rows, err := st.Automation().ListEmitAttempts(ctx, 12, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) > 50 { // limit clamps to the default when out of range
		t.Fatalf("list should clamp, got %d", len(rows))
	}
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM templateemitattempt WHERE template_id = 12`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 200 {
		t.Fatalf("want log trimmed to 200 rows, got %d", n)
	}
}

// A failed run schedules its retry; ticks before that must not run the command
// again (no new attempt, no new log row), the first tick after it must.
func TestEmitPoolRetryPauseIsHonored(t *testing.T) {
	st := openEmitPoolDB(t)
	ctx := bg()
	rng := rand.New(rand.NewSource(1))
	tmpl := poolTemplate(20, `exit 9`, 1)
	now := time.Now()

	if _, _, _, err := resolveEmitPool(t, ctx, st, tmpl, "p", now, rng); err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{0, 15 * time.Second, 45 * time.Second, emitPoolRetryDelays[0] - time.Second} {
		if _, _, _, err := resolveEmitPool(t, ctx, st, tmpl, "p", now.Add(d), rng); err != nil {
			t.Fatal(err)
		}
	}
	var attempts, runs int
	if err := st.DB.QueryRow(`SELECT attempts FROM templateemitroll WHERE template_id = 20`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM templateemitattempt WHERE template_id = 20`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || runs != 1 {
		t.Fatalf("pause ignored: attempts=%d log rows=%d, want 1/1", attempts, runs)
	}

	if _, _, _, err := resolveEmitPool(t, ctx, st, tmpl, "p", now.Add(emitPoolRetryDelays[0]+time.Second), rng); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT attempts FROM templateemitroll WHERE template_id = 20`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("retry after the pause should run: attempts=%d, want 2", attempts)
	}
}

// A success after a failure clears the pending retry.
func TestEmitPoolSuccessClearsRetryAt(t *testing.T) {
	st := openEmitPoolDB(t)
	ctx := bg()
	rng := rand.New(rand.NewSource(1))
	now := time.Now()
	if _, _, _, err := resolveEmitPool(t, ctx, st, poolTemplate(21, `exit 1`, 1), "p", now, rng); err != nil {
		t.Fatal(err)
	}
	ok := poolTemplate(21, `echo '{"title":"a"}'`, 1)
	if _, _, _, err := resolveEmitPool(t, ctx, st, ok, "p", now.Add(emitPoolRetryDelays[0]+time.Second), rng); err != nil {
		t.Fatal(err)
	}
	var retry *string
	if err := st.DB.QueryRow(`SELECT retry_at FROM templateemitroll WHERE template_id = 21`).Scan(&retry); err != nil {
		t.Fatal(err)
	}
	if retry != nil {
		t.Fatalf("retry_at should be cleared after a success, got %q", *retry)
	}
}
