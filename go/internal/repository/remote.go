package repository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
	result, err := h.Egress.Do(r.Context(), egress.Request{Workspace: wsctx.From(r.Context()), SameHostOnly: true, Method: "GET", URL: endpoint, Header: headers})
	if err != nil {
		return err
	}
	if result.Status != 200 {
		return fmt.Errorf("хост репозитория вернул HTTP %d", result.Status)
	}
	if err = json.Unmarshal(result.Body, out); err != nil {
		return fmt.Errorf("не удалось прочитать ответ хоста")
	}
	return nil
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
				v.Tree = append(v.Tree, Entry{e.Name, e.Path, kind, e.SHA})
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
		v.Tree = append(v.Tree, Entry{e.Name, e.Path, kind, e.ID})
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
