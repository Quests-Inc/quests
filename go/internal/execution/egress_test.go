package execution

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// openWithOwner is openSecretsDB plus a store on the same database as the owner role, which is the only one that writes grants.
func openWithOwner(t *testing.T) (app, owner *Store) {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	pgtest.Seed(t, pool, secretsTestSeed)
	return New(pool, nil), New(pgtest.Owner(t, url), nil)
}

func TestEgressGrantLogAndPurge(t *testing.T) {
	st, owner := openWithOwner(t)
	ctx := wsctx.With(context.Background(), 1)
	if st.EgressEnabled(ctx) {
		t.Fatal("off by default")
	}
	if err := st.SetEgressGrant(ctx, 1, true); err == nil {
		t.Fatal("the API role must not write grants")
	}
	if err := owner.SetEgressGrant(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if !st.EgressEnabled(ctx) {
		t.Fatal("granted")
	}
	grants, err := owner.EgressGrants(ctx)
	if err != nil || !grants[1] {
		t.Fatalf("grants: %v %v", grants, err)
	}
	if err := owner.SetEgressGrant(ctx, 1, false); err != nil || st.EgressEnabled(ctx) {
		t.Fatalf("revoked: %v", err)
	}

	st.LogEgress(ctx, egress.Entry{Workspace: 1, ScriptID: 7, Host: "a.example", Method: "GET", Status: 200, Bytes: 5, Millis: 3, Result: "ok"})
	st.LogEgress(ctx, egress.Entry{Workspace: 1, Host: "b.example", Method: "GET", Result: "denied: x"})
	lines, err := st.RecentEgress(ctx, 1, 10)
	if err != nil || len(lines) != 2 || lines[0].Host != "b.example" || lines[1].ScriptID != 7 {
		t.Fatalf("log: %+v %v", lines, err)
	}
	if lines, _ = st.RecentEgress(ctx, 0, 0); len(lines) != 2 {
		t.Fatalf("all workspaces: %+v", lines)
	}
	n, err := st.PurgeEgress(ctx, time.Now().Add(time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("purge: %d %v", n, err)
	}
}

func fetchFailure(t *testing.T, err error) *graph.FetchFailure {
	t.Helper()
	var f *graph.FetchFailure
	if !errors.As(err, &f) {
		t.Fatalf("want FetchFailure, got %v", err)
	}
	return f
}

func TestFetcherPolicyGrantAndAnswer(t *testing.T) {
	st, owner := openWithOwner(t)
	ctx := wsctx.With(context.Background(), 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"a":1}`))
		case "/bin":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("x"))
		}
	}))
	defer srv.Close()

	if _, err := st.NewFetcher(nil, 1)(ctx, graph.FetchRequest{URL: srv.URL}); fetchFailure(t, err).Kind != graph.FetchDenied {
		t.Fatal("no client: denied")
	}
	off := egress.New(egress.Policy{Mode: egress.ModeOff}, nil)
	if _, err := st.NewFetcher(off, 1)(ctx, graph.FetchRequest{URL: srv.URL}); fetchFailure(t, err).Kind != graph.FetchDenied {
		t.Fatal("policy off: denied")
	}

	p, err := egress.PolicyFromEnv(func(k string) string {
		return map[string]string{"QUESTS_EGRESS_MODE": "public", "QUESTS_EGRESS_ALLOW_NETS": "127.0.0.0/8", "QUESTS_EGRESS_PORTS": "80,443," + portOf(srv.URL)}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	c := egress.New(p, func(e egress.Entry) { st.LogEgress(ctx, e) })
	f := st.NewFetcher(c, 9)

	if _, err := f(ctx, graph.FetchRequest{URL: srv.URL + "/json"}); fetchFailure(t, err).Kind != graph.FetchDenied {
		t.Fatal("workspace without grant: denied")
	}
	if err := owner.SetEgressGrant(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	res, err := f(ctx, graph.FetchRequest{URL: srv.URL + "/json"})
	if err != nil || res.Status != 200 || string(res.Body) != `{"a":1}` {
		t.Fatalf("answer: %+v %v", res, err)
	}
	if _, err := f(ctx, graph.FetchRequest{URL: srv.URL + "/bin"}); fetchFailure(t, err).Kind != graph.FetchBadContent {
		t.Fatal("binary answer refused")
	}
	if _, err := f(ctx, graph.FetchRequest{URL: srv.URL + "/json", Auth: &graph.FetchAuth{Header: "X", Secret: "k"}}); fetchFailure(t, err).Kind != graph.FetchDenied {
		t.Fatal("secret header without secrets input")
	}
	if _, err := f(ctx, graph.FetchRequest{URL: srv.URL + "/json", Auth: &graph.FetchAuth{Header: "X", Secret: "k"}, SecretsOf: graph.Ref{Kind: "workspace"}}); fetchFailure(t, err).Kind != graph.FetchError {
		t.Fatal("missing secret")
	}
	if _, err := f(ctx, graph.FetchRequest{URL: "http://127.0.0.1:1/x"}); err == nil {
		t.Fatal("closed port fails")
	}
	if lines, _ := st.RecentEgress(ctx, 1, 20); len(lines) == 0 {
		t.Fatal("requests are logged")
	}
}

func portOf(u string) string {
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] == ':' {
			return u[i+1:]
		}
	}
	return ""
}

func TestHookDisableEnabledAndAncestors(t *testing.T) {
	st := openSecretsDB(t)
	ctx := wsctx.With(context.Background(), 1)
	sc, err := st.CreateScript(ctx, "s", "", questHookGraph())
	if err != nil {
		t.Fatal(err)
	}
	h, err := st.CreateHook(ctx, "h", []string{"complete"}, sc.ID, nil, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	on, err := st.EnabledHooks(ctx, "quest_completed")
	if err != nil || len(on) != 1 {
		t.Fatalf("enabled: %+v %v", on, err)
	}
	if err := st.DisableHook(ctx, h.ID, "шторм"); err != nil {
		t.Fatal(err)
	}
	if on, _ = st.EnabledHooks(ctx, "quest_completed"); len(on) != 0 {
		t.Fatalf("disabled: %+v", on)
	}
	got, _ := st.GetHook(ctx, h.ID)
	if got.Enabled || got.DisabledReason != "шторм" {
		t.Fatalf("reason: %+v", got)
	}

	root := insertNote(t, st, "root", nil, false)
	mid := insertNote(t, st, "mid", &root, false)
	leaf := insertNote(t, st, "leaf", &mid, false)
	if a := st.NoteAncestors(ctx, leaf); len(a) != 2 || a[0] != mid || a[1] != root {
		t.Fatalf("ancestors: %v", a)
	}
	if n, err := st.CountOpenEmbedded(ctx); err != nil || n != 0 {
		t.Fatalf("open embedded: %d %v", n, err)
	}
}
