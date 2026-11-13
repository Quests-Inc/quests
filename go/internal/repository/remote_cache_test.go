package repository

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/egress"
)

func TestRemoteCacheRefreshAndCredentialIsolation(t *testing.T) {
	key := remoteKey{Root: t.TempDir(), Workspace: 1, Repository: 1, URL: "https://github.com/o/r", Credential: "first", Endpoint: "https://api.github.com/test"}
	calls := 0
	fetch := func(etag string) (*egress.Result, error) {
		calls++
		if calls > 1 && etag == `"v1"` {
			return &egress.Result{Status: 304}, nil
		}
		return &egress.Result{Status: 200, Body: []byte(`{"value":1}`), ETag: `"v1"`}, nil
	}
	var out map[string]int
	for i := 0; i < 2; i++ {
		if err := cachedRemote(context.Background(), key, false, fetch, &out); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("cache miss", calls)
	}
	if err := cachedRemote(context.Background(), key, true, fetch, &out); err != nil || out["value"] != 1 || calls != 2 {
		t.Fatal("conditional refresh", out, calls, err)
	}
	key.Credential = "new"
	if err := cachedRemote(context.Background(), key, false, fetch, &out); err != nil || calls != 3 {
		t.Fatal("credential cache crossed", calls, err)
	}
	key.Workspace = 2
	if err := cachedRemote(context.Background(), key, false, fetch, &out); err != nil || calls != 4 {
		t.Fatal("workspace cache crossed", calls, err)
	}
}
func TestRemoteCacheCoalescesAndRetriesFailures(t *testing.T) {
	key := remoteKey{Root: t.TempDir(), Endpoint: "https://api.github.com/test"}
	var calls atomic.Int32
	fetch := func(string) (*egress.Result, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &egress.Result{Status: 200, Body: []byte(`[]`)}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out []string
			if err := cachedRemote(context.Background(), key, false, fetch, &out); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate parallel fetches", calls.Load())
	}
	key.Endpoint += "/failure"
	for i := 0; i < 2; i++ {
		var out any
		_ = cachedRemote(context.Background(), key, false, func(string) (*egress.Result, error) { calls.Add(1); return nil, errors.New("offline") }, &out)
	}
	if calls.Load() != 3 {
		t.Fatal("errors cached")
	}
}
func TestRemoteCacheRateLimitStopsOtherPaths(t *testing.T) {
	key := remoteKey{Root: t.TempDir(), Endpoint: "https://api.github.com/a"}
	calls := 0
	fetch := func(string) (*egress.Result, error) {
		calls++
		return &egress.Result{Status: 429, RetryAfter: "120"}, nil
	}
	var out any
	if err := cachedRemote(context.Background(), key, false, fetch, &out); err == nil || !strings.Contains(err.Error(), "лимит") {
		t.Fatal(err)
	}
	key.Endpoint = "https://api.github.com/b"
	_ = cachedRemote(context.Background(), key, true, fetch, &out)
	if calls != 1 {
		t.Fatal("retried during cooldown")
	}
}

type recursiveRemote struct {
	fakeRemote
	oversized bool
}

func (f *recursiveRemote) Do(ctx context.Context, in egress.Request) (*egress.Result, error) {
	if strings.Contains(in.URL, "/git/trees/") {
		f.calls = append(f.calls, in.URL)
		if f.oversized {
			return nil, egress.ErrTooLarge
		}
		return &egress.Result{Status: 200, Body: []byte(`{"tree":[{"path":"src","type":"tree","sha":"one"},{"path":"src/main.go","type":"blob","sha":"two"}],"truncated":false}`)}, nil
	}
	return f.fakeRemote.Do(ctx, in)
}
func TestRecursiveTreeReusedAcrossDirectories(t *testing.T) {
	client := &recursiveRemote{}
	h := Handler{DataDir: t.TempDir(), Egress: client, RemoteEnabled: func(context.Context) bool { return true }}
	repo := Record{Kind: "github", URL: "https://github.com/o/r"}
	r := httptest.NewRequest("GET", "/", nil)
	root, nested := View{}, View{}
	if err := h.remoteView(r, repo, "main", "", &root); err != nil {
		t.Fatal(err)
	}
	count := len(client.calls)
	if err := h.remoteView(r, repo, "main", "src", &nested); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != count {
		t.Fatal("directory navigation fetched the recursive tree again", client.calls)
	}
	refreshed := View{}
	if err := h.remoteView(httptest.NewRequest("GET", "/?refresh=1", nil), repo, "main", "src", &refreshed); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != count+2 {
		t.Fatal("refresh should check heads and commit list, not immutable tree", client.calls)
	}
	if len(root.Tree) != 1 || root.Tree[0].Kind != "directory" || len(nested.Tree) != 1 || nested.Tree[0].Path != "src/main.go" {
		t.Fatalf("recursive reuse: %v %+v %+v", client.calls, root, nested)
	}
}
func TestOversizedRecursiveTreeFallsBackWithoutRepeatedProbe(t *testing.T) {
	client := &recursiveRemote{oversized: true}
	h := Handler{DataDir: t.TempDir(), Egress: client, RemoteEnabled: func(context.Context) bool { return true }}
	repo := Record{Kind: "github", URL: "https://github.com/o/r"}
	r := httptest.NewRequest("GET", "/", nil)
	for _, path := range []string{"", "src"} {
		v := View{}
		if err := h.remoteView(r, repo, "main", path, &v); err != nil || len(v.Tree) != 1 {
			t.Fatal(err, v)
		}
	}
	count := 0
	for _, call := range client.calls {
		if strings.Contains(call, "/git/trees/") {
			count++
		}
	}
	if count != 1 {
		t.Fatal("oversized response probed again", count)
	}
}
