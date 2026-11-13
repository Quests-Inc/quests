package repository

import (
	"context"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/egress"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRemote struct{ calls []string }

func (f *fakeRemote) Do(_ context.Context, in egress.Request) (*egress.Result, error) {
	if in.Budget != egress.BudgetRepositories {
		return nil, fmt.Errorf("repository used script budget")
	}
	f.calls = append(f.calls, in.URL)
	var body string
	switch {
	case strings.Contains(in.URL, "/branches"):
		body = `[{"name":"main"}]`
	case strings.Contains(in.URL, "/commits"):
		if strings.Contains(in.URL, "github") {
			body = `[{"sha":"abcd","commit":{"message":"Hello","author":{"name":"A","date":"2026-11-13"},"committer":{"date":"2026-11-14"}}}]`
		} else {
			body = `[{"id":"abcd","title":"Hello","author_name":"A","created_at":"2026-11-13","committed_date":"2026-11-14"}]`
		}
	case strings.Contains(in.URL, "/contents/"):
		if strings.Contains(in.URL, "hello.txt") {
			body = `{"name":"hello.txt","path":"hello.txt","type":"file","encoding":"base64","content":"aGVsbG8K"}`
		} else {
			body = `[{"name":"hello.txt","path":"hello.txt","type":"file","sha":"abcd"}]`
		}
	case strings.Contains(in.URL, "/files/"):
		body = `{"encoding":"base64","content":"aGVsbG8K"}`
	case strings.Contains(in.URL, "/tree"):
		body = `[{"name":"hello.txt","path":"hello.txt","type":"blob","id":"abcd"}]`
	default:
		return nil, fmt.Errorf("unexpected %s", in.URL)
	}
	return &egress.Result{Status: 200, Body: []byte(body)}, nil
}
func TestReadProviderViews(t *testing.T) {
	for _, kind := range []string{"github", "gitlab"} {
		t.Run(kind, func(t *testing.T) {
			client := &fakeRemote{}
			h := Handler{DataDir: t.TempDir(), Egress: client, RemoteEnabled: func(context.Context) bool { return true }}
			r := httptest.NewRequest("GET", "/", nil)
			repo := Record{Kind: kind, URL: "https://" + kind + ".com/team/project"}
			v := View{}
			if err := h.remoteView(r, repo, "main", "", &v); err != nil {
				t.Fatal(err)
			}
			if len(v.Branches) != 1 || len(v.Commits) != 1 || len(v.Tree) != 1 {
				t.Fatalf("bad view %+v", v)
			}

			h.remoteEntryCommits(r, repo, &v)
			if v.Tree[0].LastCommit == nil || v.Tree[0].LastCommit.Title != "Hello" || v.Tree[0].LastCommit.Date != "2026-11-14" {
				t.Fatalf("missing path commit: %+v", v.Tree)
			}
			if last := client.calls[len(client.calls)-1]; !strings.Contains(last, "path=hello.txt") || !strings.Contains(last, "abcd") {
				t.Fatal(last)
			}

			count := len(client.calls)
			v.Tree[0].LastCommit = nil
			h.remoteEntryCommits(r, repo, &v)
			if len(client.calls) != count || v.Tree[0].LastCommit == nil {
				t.Fatal("unchanged commit metadata was fetched again")
			}

			for _, call := range client.calls {
				if !strings.HasPrefix(call, "https://api.github.com/repos/") && !strings.HasPrefix(call, "https://gitlab.com/api/v4/projects/") {
					t.Fatal(call)
				}
			}
		})
	}
}
