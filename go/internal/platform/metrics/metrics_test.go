package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/health"
	"github.com/valirum/quests/go/internal/platform/pgtest"
	"github.com/valirum/quests/go/internal/store"
)

func testMetrics(t *testing.T) *Metrics {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	for _, q := range []string{
		`INSERT INTO quest (title, description, status, pinned, sort_order, created_at, updated_at)
			VALUES ('a', '', 'active', 0, 0, now(), now()), ('b', '', 'active', 0, 0, now(), now()), ('c', '', 'completed', 0, 0, now(), now())`,
		`INSERT INTO runner_job (runner_label, kind, command, timeout_seconds, valid_until, created_at) VALUES ('r', 'check', 'true', 60, now(), now())`,
		`INSERT INTO backuplog (filename, size_bytes, created_at, webdav_uploaded, remote_uploaded) VALUES ('b.db', 1234, '2026-10-08 12:00:00.000000', 0, 0)`,
	} {
		if _, err := pool.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return New(pool, storeStats{&store.Store{DB: pool, DatabaseURL: url}}, events.New(), health.New())
}

// storeStats is what the server does in cmd/quests-server: the real store behind the StatsSource port.
type storeStats struct{ st *store.Store }

func (s storeStats) AggregateStats(ctx context.Context) (Stats, error) {
	st, err := s.st.AggregateStats(ctx)
	return Stats(st), err
}

func get(h http.Handler, token string) (int, string) {
	req := httptest.NewRequest("GET", "/metrics", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(b)
}

func TestMetricsEndpointIsTokenGuarded(t *testing.T) {
	m := testMetrics(t)
	if code, _ := get(m.Handler(""), "anything"); code != http.StatusNotFound {
		t.Fatalf("no token configured: %d, want 404", code)
	}
	h := m.Handler("s3cret")
	if code, _ := get(h, ""); code != http.StatusUnauthorized {
		t.Fatalf("no credentials: %d, want 401", code)
	}
	if code, _ := get(h, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d, want 401", code)
	}
	code, body := get(h, "s3cret")
	if code != 200 {
		t.Fatalf("right token: %d", code)
	}
	for _, want := range []string{
		`quests_quests{status="active"} 2`, `quests_quests{status="completed"} 1`,
		`quests_runner_jobs{state="queued"} 1`, `quests_backup_last_size_bytes 1234`,
		"quests_workspaces 1", "go_goroutines", "quests_stats_collect_ok 1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in /metrics", want)
		}
	}
}

func TestMiddlewareLabelsByRoutePatternNotPath(t *testing.T) {
	m := testMetrics(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/quests/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	h := m.Middleware(mux)
	for _, p := range []string{"/api/quests/1", "/api/quests/2", "/api/quests/3"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	_, body := get(m.Handler("t"), "t")
	if !strings.Contains(body, `quests_http_requests_total{code="404",method="GET",route="GET /api/quests/{id}"} 3`) {
		t.Fatalf("expected one series for the route pattern, got:\n%s", grep(body, "quests_http_requests_total"))
	}
}

func grep(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
