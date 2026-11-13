package repository

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

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
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type View struct {
	Limited  bool     `json:"limited"`
	CloneURL string   `json:"clone_url"`
	Branch   string   `json:"branch"`
	Branches []string `json:"branches"`
	Commits  []Commit `json:"commits"`
	Tree     []Entry  `json:"tree"`
	File     *string  `json:"file,omitempty"`
	Binary   bool     `json:"binary"`
	Empty    bool     `json:"empty"`
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
	if !safeRef(branch) || !safePath(path) {
		fail(w, 400, fmt.Errorf("некорректная ветка или путь"))
		return
	}
	view := View{CloneURL: h.cloneURL(repo), Branch: branch, Branches: []string{}, Commits: []Commit{}, Tree: []Entry{}}
	if repo.Kind != "local" {
		err = h.remoteView(r, repo, branch, path, &view)
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
			object := commit
			if path != "" {
				object += ":" + path
			}
			var kind []byte
			if err == nil {
				kind, err = Git(r.Context(), dir, "cat-file", "-t", object)
			}
			if err == nil && strings.TrimSpace(string(kind)) == "blob" {
				raw, err = Git(r.Context(), dir, "cat-file", "blob", object)
				if err == nil {
					if utf8.Valid(raw) && !strings.ContainsRune(string(raw), 0) {
						text := string(raw)
						view.File = &text
					} else {
						view.Binary = true
					}
				}
			} else if err == nil {
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
						view.Tree = append(view.Tree, Entry{name, p, kind, parts[2]})
					}
				}
			}
		}
	}
	if err != nil {
		fail(w, 502, err)
		return
	}
	httpx.WriteJSON(w, 200, view)
}
