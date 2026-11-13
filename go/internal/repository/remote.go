package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"sync"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func validateRemote(kind, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("некорректный адрес репозитория")
	}
	host := ""
	switch kind {
	case "github":
		host = "github.com"
	case "gitlab":
		host = "gitlab.com"
	default:
		return fmt.Errorf("выберите свой Git, GitHub или GitLab")
	}
	if u.Scheme != "https" || u.Host != host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return fmt.Errorf("нужен HTTPS-адрес %s без логина и параметров", host)
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) < 2 || kind == "github" && len(parts) != 2 {
		return fmt.Errorf("адрес должен указывать на репозиторий")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsAny(p, " :\\\x00\r\n") {
			return fmt.Errorf("некорректный путь репозитория")
		}
	}
	return nil
}
func (h *Handler) remoteJSON(r *http.Request, repo Record, endpoint string, out any) error {
	if h.RemoteEnabled == nil || !h.RemoteEnabled(r.Context()) {
		return fmt.Errorf("администратор не разрешил исходящие запросы для этого пространства")
	}
	if h.Egress == nil {
		return fmt.Errorf("исходящие запросы отключены администратором")
	}
	headers := map[string]string{"Accept": "application/json", "User-Agent": "Quests-repositories"}
	var sealed string
	if repo.HasToken {
		if err := h.DB.QueryRowContext(r.Context(), `SELECT value FROM repository_token WHERE repository_id=?`, repo.ID).Scan(&sealed); err != nil {
			return fmt.Errorf("секрет репозитория недоступен")
		}
	}
	if sealed != "" {
		if h.Box == nil {
			return fmt.Errorf("секрет недоступен")
		}
		token, err := h.Box.Open(sealed)
		if err != nil {
			return err
		}
		if repo.Kind == "github" {
			headers["Authorization"] = "Bearer " + token
		} else {
			headers["PRIVATE-TOKEN"] = token
		}
	}
	key := remoteKey{Root: h.DataDir, Workspace: wsctx.From(r.Context()), Repository: repo.ID, URL: repo.URL, Credential: credentialID(headers["Authorization"] + headers["PRIVATE-TOKEN"]), Endpoint: endpoint}
	// Only branch heads/history lists are mutable. Refreshing a SHA must not
	// spend another request on its unchanged tree or per-path history.
	endpointURL, _ := url.Parse(endpoint)
	mutable := strings.HasSuffix(endpointURL.Path, "/branches") || strings.HasSuffix(endpointURL.Path, "/commits") && endpointURL.Query().Get("per_page") == "30"
	return cachedRemote(r.Context(), key, mutable && r.URL.Query().Get("refresh") == "1", func(etag string) (*egress.Result, error) {
		if etag != "" {
			headers["If-None-Match"] = etag
		}
		result, err := h.Egress.Do(r.Context(), egress.Request{Workspace: wsctx.From(r.Context()), SameHostOnly: true, Method: "GET", URL: endpoint, Header: headers})
		if errors.Is(err, egress.ErrTooLarge) && strings.Contains(endpoint, "/git/trees/") {
			return &egress.Result{Status: 200, Body: []byte(`{"truncated":true}`)}, nil
		}
		return result, err
	}, out)
}
func containsRateLimit(body []byte) bool {
	return strings.Contains(strings.ToLower(string(body)), "rate limit")
}
func (h *Handler) remoteView(r *http.Request, repo Record, branch, path string, v *View) error {
	if err := validateRemote(repo.Kind, repo.URL); err != nil {
		return err
	}
	ref := branch
	if selected := r.URL.Query().Get("commit"); selected != "" {
		ref = selected
		v.SelectedCommit = selected
	}
	u, _ := url.Parse(repo.URL)
	project := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	if repo.Kind == "github" {
		base := "https://api.github.com/repos/" + project
		var branches []struct {
			Name string `json:"name"`
		}
		if err := h.remoteJSON(r, repo, base+"/branches?per_page=100", &branches); err != nil {
			return err
		}
		if len(branches) >= 100 {
			v.Limited = true
		}
		for _, b := range branches {
			v.Branches = append(v.Branches, b.Name)
		}
		if len(branches) == 0 {
			v.Empty = true
			return nil
		}
		var commits []struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
				Author  struct {
					Name string `json:"name"`
					Date string `json:"date"`
				} `json:"author"`
			} `json:"commit"`
		}
		if err := h.remoteJSON(r, repo, base+"/commits?per_page=30&sha="+url.QueryEscape(branch), &commits); err != nil {
			return err
		}
		for _, c := range commits {
			title, _, _ := strings.Cut(c.Commit.Message, "\n")
			v.Commits = append(v.Commits, Commit{c.SHA, title, c.Commit.Author.Name, c.Commit.Author.Date})
		}
		if v.SelectedCommit == "" && len(v.Commits) > 0 {
			v.SelectedCommit = v.Commits[0].ID
		}
		if v.SelectedCommit != "" {
			ref = v.SelectedCommit
		}
		// A recursive immutable tree costs one request for all directories. A
		// truncated/oversized answer falls back to the normal directory endpoint.
		var recursive struct {
			Truncated bool `json:"truncated"`
			Tree      []struct {
				Path string `json:"path"`
				Type string `json:"type"`
				SHA  string `json:"sha"`
				Mode string `json:"mode"`
			} `json:"tree"`
		}
		if err := h.remoteJSON(r, repo, base+"/git/trees/"+url.PathEscape(ref)+"?recursive=1", &recursive); err == nil && !recursive.Truncated {
			directory := path
			foundDirectory := path == ""
			for _, entry := range recursive.Tree {
				if entry.Path == directory && entry.Type == "tree" {
					foundDirectory = true
				}
				parent := pathpkg.Dir(entry.Path)
				if parent == "." {
					parent = ""
				}
				if parent != directory {
					continue
				}
				kind := "file"
				if entry.Type == "tree" {
					kind = "directory"
				} else if entry.Mode == "120000" {
					kind = "symlink"
				} else if entry.Type == "commit" {
					kind = "submodule"
				}
				v.Tree = append(v.Tree, Entry{Name: pathpkg.Base(entry.Path), Path: entry.Path, Kind: kind, ID: entry.SHA})
			}
			if !foundDirectory {
				return fmt.Errorf("выберите каталог; просмотр файлов пока не включён")
			}
			return nil
		}
		endpoint := base + "/contents/" + strings.Join(escapeParts(path), "/") + "?ref=" + url.QueryEscape(ref)
		var raw json.RawMessage
		if err := h.remoteJSON(r, repo, endpoint, &raw); err != nil {
			return err
		}
		type content struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			Type     string `json:"type"`
			SHA      string `json:"sha"`
			Encoding string `json:"encoding"`
			Content  string `json:"content"`
		}
		if len(raw) > 0 && raw[0] == '[' {
			var items []content
			if err := json.Unmarshal(raw, &items); err != nil {
				return err
			}
			if len(items) >= 100 {
				v.Limited = true
			}
			for _, e := range items {
				kind := "file"
				if e.Type == "dir" {
					kind = "directory"
				}
				v.Tree = append(v.Tree, Entry{Name: e.Name, Path: e.Path, Kind: kind, ID: e.SHA})
			}
		} else {
			return fmt.Errorf("выберите каталог; просмотр файлов пока не включён")
		}

		return nil
	}
	base := "https://gitlab.com/api/v4/projects/" + url.PathEscape(project) + "/repository"
	var branches []struct {
		Name string `json:"name"`
	}
	if err := h.remoteJSON(r, repo, base+"/branches?per_page=100", &branches); err != nil {
		return err
	}
	if len(branches) >= 100 {
		v.Limited = true
	}
	for _, b := range branches {
		v.Branches = append(v.Branches, b.Name)
	}
	if len(branches) == 0 {
		v.Empty = true
		return nil
	}
	var commits []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Author string `json:"author_name"`
		Date   string `json:"created_at"`
	}
	if err := h.remoteJSON(r, repo, base+"/commits?per_page=30&ref_name="+url.QueryEscape(branch), &commits); err != nil {
		return err
	}
	for _, c := range commits {
		v.Commits = append(v.Commits, Commit{c.ID, c.Title, c.Author, c.Date})
	}

	if v.SelectedCommit == "" && len(v.Commits) > 0 {
		v.SelectedCommit = v.Commits[0].ID
	}
	if v.SelectedCommit != "" {
		ref = v.SelectedCommit
	}
	var items []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := h.remoteJSON(r, repo, base+"/tree?per_page=100&ref="+url.QueryEscape(ref)+"&path="+url.QueryEscape(path), &items); err != nil {
		return err
	}
	if len(items) >= 100 {
		v.Limited = true
	}
	for _, e := range items {
		kind := "file"
		if e.Type == "tree" {
			kind = "directory"
		}
		v.Tree = append(v.Tree, Entry{Name: e.Name, Path: e.Path, Kind: kind, ID: e.ID})
	}
	return nil
}
func escapeParts(path string) []string {
	if path == "" {
		return []string{}
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return parts
}

// Host APIs expose path history separately. Bound concurrent requests and total
// latency; missing metadata must not make the directory itself unavailable.
func (h *Handler) remoteEntryCommits(r *http.Request, repo Record, view *View) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	u, _ := url.Parse(repo.URL)
	project := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	var wg sync.WaitGroup
	slots := make(chan struct{}, 2)

	for i := range view.Tree {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			view.Limited = true
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() { <-slots }()
			entry := &view.Tree[index]
			if repo.Kind == "github" {
				var commits []struct {
					SHA    string `json:"sha"`
					Commit struct {
						Message string `json:"message"`
						Author  struct {
							Name string `json:"name"`
						} `json:"author"`
						Committer struct {
							Date string `json:"date"`
						} `json:"committer"`
					} `json:"commit"`
				}
				endpoint := "https://api.github.com/repos/" + project + "/commits?per_page=1&sha=" + url.QueryEscape(view.SelectedCommit) + "&path=" + url.QueryEscape(entry.Path)
				if h.remoteJSON(r, repo, endpoint, &commits) == nil && len(commits) > 0 {
					c := commits[0]
					title, _, _ := strings.Cut(c.Commit.Message, "\n")
					entry.LastCommit = &Commit{c.SHA, title, c.Commit.Author.Name, c.Commit.Committer.Date}
				}
			} else {
				var commits []struct {
					ID     string `json:"id"`
					Title  string `json:"title"`
					Author string `json:"author_name"`
					Date   string `json:"committed_date"`
				}
				endpoint := "https://gitlab.com/api/v4/projects/" + url.PathEscape(project) + "/repository/commits?per_page=1&ref_name=" + url.QueryEscape(view.SelectedCommit) + "&path=" + url.QueryEscape(entry.Path)
				if h.remoteJSON(r, repo, endpoint, &commits) == nil && len(commits) > 0 {
					c := commits[0]
					entry.LastCommit = &Commit{c.ID, c.Title, c.Author, c.Date}
				}
			}
		}(i)
	}
	wg.Wait()
	for _, entry := range view.Tree {
		if entry.LastCommit == nil {
			view.Limited = true
		}
	}
}
