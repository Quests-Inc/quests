package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"github.com/quests-inc/quests/go/internal/repository"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRepositoriesGitHTTPAndWorkspace(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	s.DataDir = t.TempDir()
	handler := s.repositoryHTTP()
	handler.Quota = 100000
	mux := http.NewServeMux()
	handler.Register(mux)
	server := httptest.NewServer(s.withWorkspace(mux))
	defer server.Close()
	call := func(method, path string, body any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(raw)))
		if w.Code != want {
			t.Fatalf("%s %s = %d %s", method, path, w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	made := call("POST", "/api/repositories", map[string]any{"title": "Tiny", "kind": "local", "slug": "tiny"}, 201)
	id := int(made["id"].(float64))
	base := "/api/repositories/" + strconv.Itoa(id)
	call("POST", "/api/repositories", map[string]any{"title": "Duplicate", "slug": "tiny"}, 409)
	call("PATCH", base, map[string]any{"slug": "Bad slug"}, 400)
	call("PATCH", base, map[string]any{"title": "Renamed"}, 200)
	if got := call("GET", base+"/browse", nil, 200)["clone_url"]; got != "/git/workspace-1/tiny.git" {
		t.Fatal(got)
	}

	if !call("GET", base+"/browse", nil, 200)["empty"].(bool) {
		t.Fatal("new repo not empty")
	}
	local := filepath.Join(t.TempDir(), "clone")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("clone", server.URL+"/git/workspace-1/tiny.git", local)
	if err := os.WriteFile(filepath.Join(local, "hello.txt"), []byte("hello source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("-C", local, "add", "hello.txt")
	git("-C", local, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-m", "First source")
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(repository.Directory(s.DataDir, 1, int64(id)), "hooks", "post-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	git("-C", local, "push", "origin", "HEAD:main")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("post-receive hook executed")
	}
	bundle := filepath.Join(t.TempDir(), "repo.bundle")
	if err := repository.Bundle(context.Background(), repository.Directory(s.DataDir, 1, int64(id)), bundle); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.git")
	if err := repository.RestoreBundle(context.Background(), restored, bundle); err != nil {
		t.Fatal(err)
	}
	if raw, err := repository.Git(context.Background(), restored, "show", "refs/heads/main:hello.txt"); err != nil || string(raw) != "hello source\n" {
		t.Fatalf("bundle roundtrip %s %v", raw, err)
	}
	old := call("GET", base+"/browse", nil, 200)["selected_commit"].(string)
	if err := os.WriteFile(filepath.Join(local, "second.txt"), []byte("second version"), 0600); err != nil {
		t.Fatal(err)
	}
	git("-C", local, "add", "second.txt")
	git("-C", local, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-m", "Second source")
	git("-C", local, "push", "origin", "HEAD:main")
	historical := call("GET", base+"/browse?commit="+old, nil, 200)
	if len(historical["tree"].([]any)) != 1 {
		t.Fatal("selected commit did not select old tree")
	}
	call("POST", base+"/links", map[string]any{"kind": "step", "id": 1}, 400)
	note := call("POST", "/api/notes", map[string]any{"title": "Repo note"}, 201)
	call("POST", base+"/links", map[string]any{"kind": "note", "id": note["id"]}, 200)
	large := make([]byte, 300000)
	if _, err := rand.Read(large); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "large.bin"), large, 0600); err != nil {
		t.Fatal(err)
	}
	git("-C", local, "add", "large.bin")
	git("-C", local, "-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-m", "Over quota")
	cmd := exec.Command("git", "-C", local, "push", "origin", "HEAD:main")
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("overquota push succeeded %s", output)
	}
	view := call("GET", base+"/browse", nil, 200)
	if len(view["commits"].([]any)) != 2 {
		t.Fatal(view)
	}
	call("GET", base+"/browse?path=hello.txt", nil, 400)
	call("GET", base+"/browse?path=../config", nil, 400)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", base+"/browse", nil)
	r.Header.Set("X-Quests-Workspace", "2")
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("cross-workspace=%d %s", w.Code, w.Body.String())
	}
}
