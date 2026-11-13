package repository

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

type Commit struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
	Date   string `json:"date"`
}
type Entry struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	Kind       string  `json:"kind"`
	ID         string  `json:"id"`
	LastCommit *Commit `json:"last_commit,omitempty"`
}
type View struct {
	SelectedCommit string   `json:"selected_commit"`
	Limited        bool     `json:"limited"`
	CloneURL       string   `json:"clone_url"`
	Branch         string   `json:"branch"`
	Branches       []string `json:"branches"`
	Commits        []Commit `json:"commits"`
	Tree           []Entry  `json:"tree"`
	Empty          bool     `json:"empty"`
}

func (h *Handler) browse(w http.ResponseWriter, r *http.Request) {
	repo, err := h.get(r)
	if err != nil {
		fail(w, 404, fmt.Errorf("репозиторий не найден"))
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = repo.Branch
	}
	path := r.URL.Query().Get("path")
	selected := r.URL.Query().Get("commit")
	if selected != "" && !validCommit(selected) {
		fail(w, 400, fmt.Errorf("некорректный коммит"))
		return
	}
	if !safeRef(branch) || !safePath(path) {
		fail(w, 400, fmt.Errorf("некорректная ветка или путь"))
		return
	}
	view := View{CloneURL: h.cloneURL(r.Context(), repo), Branch: branch, Branches: []string{}, Commits: []Commit{}, Tree: []Entry{}}
	if repo.Kind != "local" {
		err = h.remoteView(r, repo, branch, path, &view)
		if err == nil && !view.Empty {
			h.remoteEntryCommits(r, repo, &view)
		}
	} else {
		lock := WorkspaceLock(h.DataDir, wsctx.From(r.Context()))
		lock.RLock()
		defer lock.RUnlock()
		dir := h.directory(wsctx.From(r.Context()), repo.ID)
		recoverDirectory(dir)
		var raw []byte
		raw, err = Git(r.Context(), dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if line != "" {
					view.Branches = append(view.Branches, line)
				}
			}
		}
		if err == nil && len(view.Branches) == 0 {
			view.Empty = true
			httpx.WriteJSON(w, 200, view)
			return
		}
		var sha []byte
		if err == nil {
			sha, err = Git(r.Context(), dir, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
		}
		if err == nil {
			commit := strings.TrimSpace(string(sha))
			raw, err = Git(r.Context(), dir, "log", "-30", "--format=%H%x00%s%x00%an%x00%aI", commit, "--")
			if err == nil {
				for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
					parts := strings.Split(line, "\x00")
					if len(parts) == 4 {
						view.Commits = append(view.Commits, Commit{parts[0], parts[1], parts[2], parts[3]})
					}
				}
			}
			if selected != "" {
				resolved, e := Git(r.Context(), dir, "rev-parse", "--verify", selected+"^{commit}")
				if e != nil {
					fail(w, 404, fmt.Errorf("коммит не найден"))
					return
				}
				commit = strings.TrimSpace(string(resolved))
			}
			view.SelectedCommit = commit
			object := commit + "^{tree}"
			if path != "" {
				object = commit + ":" + path
			}
			var kind []byte
			if err == nil {
				kind, err = Git(r.Context(), dir, "cat-file", "-t", object)
			}
			if err == nil && strings.TrimSpace(string(kind)) != "tree" {
				fail(w, 400, fmt.Errorf("выберите каталог; просмотр файлов пока не включён"))
				return
			}
			if err == nil {
				raw, err = Git(r.Context(), dir, "ls-tree", "-z", object)
				if err == nil {
					for _, row := range strings.Split(string(raw), "\x00") {
						meta, name, ok := strings.Cut(row, "\t")
						parts := strings.Fields(meta)
						if !ok || len(parts) != 3 {
							continue
						}
						p := name
						if path != "" {
							p = path + "/" + name
						}
						kind := "file"
						if parts[1] == "tree" {
							kind = "directory"
						}
						if parts[0] == "120000" {
							kind = "symlink"
						}
						if parts[0] == "160000" {
							kind = "submodule"
						}
						view.Tree = append(view.Tree, Entry{Name: name, Path: p, Kind: kind, ID: parts[2]})
					}
				}
			}
		}
	}
	if err == nil && repo.Kind == "local" && len(view.Tree) > 0 {
		view.Limited = !localEntryCommits(r.Context(), h.directory(wsctx.From(r.Context()), repo.ID), view.SelectedCommit, path, view.Tree)
	}
	if err != nil {
		fail(w, 502, err)
		return
	}
	httpx.WriteJSON(w, 200, view)
}

func validCommit(raw string) bool {
	if len(raw) != 40 && len(raw) != 64 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}

// Scan history once for the whole directory, rather than spawning git per row.
// The bounded scan never substitutes the branch tip for an unknown file date.
func localEntryCommits(ctx context.Context, dir, ref, path string, entries []Entry) bool {
	args := []string{"log", "-1000", "-m", "--format=%x00%H%x00%s%x00%an%x00%cI%x00", "--name-only", "-z", "--no-renames", ref, "--"}
	if path != "" {
		args = append(args, ":(literal)"+path)
	}
	raw, err := Git(ctx, dir, args...)
	if err != nil {
		return false
	}
	pending := make(map[string]int, len(entries))
	for i, e := range entries {
		pending[e.Path] = i
	}
	tokens := strings.Split(string(raw), "\x00")
	for i := 0; i < len(tokens); {
		for i < len(tokens) && tokens[i] == "" {
			i++
		}
		if i+4 >= len(tokens) || !validCommit(tokens[i]) {
			break
		}
		c := Commit{tokens[i], tokens[i+1], tokens[i+2], tokens[i+3]}
		i += 4
		if tokens[i] == "" {
			i++
		}
		first := true
		for i < len(tokens) && tokens[i] != "" {
			name := tokens[i]
			i++
			if first {
				name = strings.TrimPrefix(name, "\n")
				first = false
			}
			if path != "" {
				if !strings.HasPrefix(name, path+"/") {
					continue
				}
				name = strings.TrimPrefix(name, path+"/")
			}
			child, _, _ := strings.Cut(name, "/")
			if path != "" {
				child = path + "/" + child
			}
			if index, ok := pending[child]; ok {
				copy := c
				entries[index].LastCommit = &copy
				delete(pending, child)
			}
		}
		if len(pending) == 0 {
			return true
		}
	}
	return len(pending) == 0
}
