package repository

import (
	"bytes"
	"context"
	"fmt"
	"github.com/quests-inc/quests/go/internal/identity"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *bufferedResponse) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	if b.body.Len()+len(p) > 4<<20 {
		return 0, fmt.Errorf("git response too large")
	}
	return b.body.Write(p)
}

func (h *Handler) transport(w http.ResponseWriter, r *http.Request) {
	repo, err := h.get(r)
	if err != nil || repo.Kind != "local" {
		fail(w, 404, fmt.Errorf("репозиторий не найден"))
		return
	}
	path := r.PathValue("path")
	service := r.URL.Query().Get("service")
	if path != "info/refs" && path != "git-upload-pack" && path != "git-receive-pack" {
		http.NotFound(w, r)
		return
	}
	if path == "info/refs" && (r.Method != "GET" || (service != "git-upload-pack" && service != "git-receive-pack")) {
		http.NotFound(w, r)
		return
	}
	if path != "info/refs" && r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	push := path == "git-receive-pack" || service == "git-receive-pack"
	if push && !h.RequireOwner(w, r, "push repositories") {
		return
	}
	lock := WorkspaceLock(h.DataDir, wsctx.From(r.Context()))
	if push {
		lock.Lock()
		defer lock.Unlock()
	} else {
		lock.RLock()
		defer lock.RUnlock()
	}
	dir := h.directory(wsctx.From(r.Context()), repo.ID)
	recoverDirectory(dir)
	projectRoot := filepath.Dir(dir)
	name := filepath.Base(dir)
	var staging string
	if path == "git-receive-pack" {
		used, e := Bytes(projectRoot)
		if e != nil {
			fail(w, 500, e)
			return
		}
		current, e := Bytes(dir)
		if e != nil {
			fail(w, 500, e)
			return
		}
		staging, err = os.MkdirTemp(filepath.Dir(projectRoot), ".push-")
		if err != nil {
			fail(w, 500, err)
			return
		}
		defer os.RemoveAll(staging) //nolint:errcheck
		if err = copyRepo(dir, filepath.Join(staging, name)); err != nil {
			fail(w, 500, err)
			return
		}
		buffer := &bufferedResponse{header: make(http.Header)}
		pushContext, cancel := context.WithCancel(r.Context())
		var over atomic.Bool
		stop := make(chan struct{})
		go func() {
			timer := time.NewTicker(100 * time.Millisecond)
			defer timer.Stop()
			for {
				select {
				case <-stop:
					return
				case <-pushContext.Done():
					return
				case <-timer.C:
					size, e := Bytes(filepath.Join(staging, name))
					if e == nil && used-current+size > h.quota() {
						over.Store(true)
						cancel()
						return
					}
				}
			}
		}()
		err = h.cgi(buffer, r.WithContext(pushContext), staging, name, path)
		close(stop)
		cancel()
		if over.Load() {
			fail(w, 413, fmt.Errorf("превышена квота репозиториев пространства"))
			return
		}
		if err != nil {
			fail(w, 502, fmt.Errorf("git HTTP failed"))
			return
		}
		after, e := Bytes(filepath.Join(staging, name))
		if e != nil {
			fail(w, 500, e)
			return
		}
		if used-current+after > h.quota() {
			fail(w, 413, fmt.Errorf("превышена квота репозиториев пространства: %d байт", h.quota()))
			return
		}
		if buffer.status >= 200 && buffer.status < 300 {
			backup := dir + ".previous"
			if err = os.Rename(dir, backup); err == nil {
				err = os.Rename(filepath.Join(staging, name), dir)
				if err != nil {
					_ = os.Rename(backup, dir)
				} else {
					_ = os.RemoveAll(backup)
				}
			}
			if err != nil {
				fail(w, 500, err)
				return
			}
		}
		actor, _ := identity.PrincipalFrom(r.Context())
		log.Printf("repository push workspace=%d repository=%d actor=%d username=%q status=%d", wsctx.From(r.Context()), repo.ID, wsctx.UserFrom(r.Context()), actor.User.Username, buffer.status)
		for key, values := range buffer.header {
			w.Header()[key] = values
		}
		w.WriteHeader(buffer.status)
		_, _ = w.Write(buffer.body.Bytes())
		return
	}
	_ = h.cgi(w, r, projectRoot, name, path)
}
func (h *Handler) cgi(w http.ResponseWriter, r *http.Request, root, name, path string) error {
	clone := r.Clone(r.Context())
	clone.Body = http.MaxBytesReader(w, r.Body, h.quota()+(1<<20))
	// Content is never interpreted as executable code. Only smart HTTP endpoints
	// are exposed; hooks, replace refs, alternates from requests and maintenance are disabled.
	username := "bootstrap"
	if principal, ok := identity.PrincipalFrom(r.Context()); ok {
		username = principal.User.Username
	}
	env := append(gitEnv(), "GIT_PROJECT_ROOT="+root, "GIT_HTTP_EXPORT_ALL=1", "PATH_INFO=/"+name+"/"+path, "REMOTE_USER="+username, "GIT_CONFIG_KEY_6=http.receivepack", "GIT_CONFIG_VALUE_6=true", "GIT_CONFIG_KEY_7=http.getanyfile", "GIT_CONFIG_VALUE_7=false")
	for i, v := range env {
		if strings.HasPrefix(v, "GIT_CONFIG_COUNT=") {
			env[i] = "GIT_CONFIG_COUNT=8"
		}
	}
	return serveGitHTTP(w, clone, root, name, path, env)
}

// Bundle writes only reachable Git objects and refs, not local hooks/config.
func Bundle(ctx context.Context, dir, target string) error {
	_, err := Git(ctx, dir, "bundle", "create", target, "--all")
	return err
}
func RestoreBundle(ctx context.Context, dir, bundle string) error {
	if err := Init(ctx, dir); err != nil {
		return err
	}
	_, err := Git(ctx, dir, "-c", "protocol.file.allow=always", "fetch", "--no-tags", bundle, "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*")
	return err
}
func (h *Handler) cloneURL(repo Record) string {
	if repo.Kind != "local" {
		return repo.URL
	}
	return "/api/repositories/" + strconv.FormatInt(repo.ID, 10) + "/git"
}

// Recover the only interruption window of replacing a staged bare directory.
// Call while holding the workspace lock, before starting a Git process.
func recoverDirectory(dir string) {
	backup := dir + ".previous"
	if _, err := os.Stat(backup); err != nil {
		return
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.Rename(backup, dir)
	} else if err == nil {
		_ = os.RemoveAll(backup)
	}
}
