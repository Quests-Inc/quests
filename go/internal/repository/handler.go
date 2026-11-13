package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

type HTTPClient interface {
	Do(context.Context, egress.Request) (*egress.Result, error)
}

type Handler struct {
	DB            *sql.DB
	DataDir       string
	Box           *secretbox.Box
	Egress        HTTPClient
	RemoteEnabled func(context.Context) bool
	RequireOwner  func(http.ResponseWriter, *http.Request, string) bool
	Quota         int64
}
type Record struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	HasToken bool   `json:"has_token"`
	Bytes    int64  `json:"bytes"`
	Links    []Link `json:"links"`
}
type Link struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

var locks sync.Map

func WorkspaceLock(root string, ws int64) *sync.RWMutex {
	v, _ := locks.LoadOrStore(fmt.Sprintf("%s/%d", root, ws), new(sync.RWMutex))
	return v.(*sync.RWMutex)
}
func (h *Handler) directory(ws, id int64) string { return Directory(h.DataDir, ws, id) }
func Directory(root string, ws, id int64) string {
	return filepath.Join(root, "repositories", strconv.FormatInt(ws, 10), strconv.FormatInt(id, 10)+".git")
}
func (h *Handler) quota() int64 {
	if h.Quota > 0 {
		return h.Quota
	}
	return DefaultQuota
}
func fail(w http.ResponseWriter, code int, err error) { httpx.WriteErr(w, code, err.Error()) }
func jsonBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, 400, fmt.Errorf("некорректные данные"))
		return false
	}
	return true
}
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/repositories", h.list)
	mux.HandleFunc("POST /api/repositories", h.create)
	mux.HandleFunc("PATCH /api/repositories/{id}", h.update)
	mux.HandleFunc("POST /api/repositories/{id}/links", h.link)
	mux.HandleFunc("DELETE /api/repositories/{id}/links", h.unlink)
	mux.HandleFunc("GET /api/repositories/{id}/browse", h.browse)
	mux.HandleFunc("GET /api/repositories/{id}/git/{path...}", h.transport)
	mux.HandleFunc("POST /api/repositories/{id}/git/{path...}", h.transport)
}
func (h *Handler) get(r *http.Request) (Record, error) {
	var repo Record
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return repo, err
	}
	err = h.DB.QueryRowContext(r.Context(), `SELECT id,title,kind,url,branch,EXISTS(SELECT 1 FROM repository_token rt WHERE rt.repository_id=repository.id) FROM repository WHERE id=?`, id).Scan(&repo.ID, &repo.Title, &repo.Kind, &repo.URL, &repo.Branch, &repo.HasToken)
	return repo, err
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `SELECT id,title,kind,url,branch,EXISTS(SELECT 1 FROM repository_token rt WHERE rt.repository_id=repository.id) FROM repository ORDER BY id`)
	if err != nil {
		fail(w, 500, err)
		return
	}
	repos := []Record{}
	for rows.Next() {
		var a Record
		if err = rows.Scan(&a.ID, &a.Title, &a.Kind, &a.URL, &a.Branch, &a.HasToken); err != nil {
			break
		}
		a.Links = []Link{}
		repos = append(repos, a)
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		fail(w, 500, err)
		return
	}
	for i := range repos {
		a := &repos[i]
		links, e := h.DB.QueryContext(r.Context(), `SELECT owner_type,owner_id,path FROM repository_link WHERE repository_id=? ORDER BY owner_type,owner_id`, a.ID)
		if e != nil {
			fail(w, 500, e)
			return
		}
		for links.Next() {
			var l Link
			if e = links.Scan(&l.Kind, &l.ID, &l.Path); e != nil {
				break
			}
			a.Links = append(a.Links, l)
		}
		if e == nil {
			e = links.Err()
		}
		_ = links.Close()
		if e != nil {
			fail(w, 500, e)
			return
		}
		if a.Kind == "local" {
			a.Bytes, _ = Bytes(h.directory(wsctx.From(r.Context()), a.ID))
		}
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": repos, "quota_bytes": h.quota()})
}

type createBody struct {
	Title  string `json:"title"`
	Kind   string `json:"kind"`
	URL    string `json:"url"`
	Branch string `json:"branch"`
	Token  string `json:"token"`
	Owner  *Link  `json:"owner"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "create repositories") {
		return
	}
	var in createBody
	if !jsonBody(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Kind == "" {
		in.Kind = "local"
	}
	if in.Branch == "" {
		in.Branch = "main"
	}
	if in.Title == "" || len(in.Title) > 200 || !safeRef(in.Branch) {
		fail(w, 400, fmt.Errorf("нужны название и корректная ветка"))
		return
	}
	if in.Kind != "local" {
		if err := validateRemote(in.Kind, in.URL); err != nil {
			fail(w, 400, err)
			return
		}
	} else {
		in.URL = ""
		in.Token = ""
	}
	if in.Owner != nil {
		if err := h.owner(r, *in.Owner); err != nil {
			fail(w, 400, err)
			return
		}
	}
	sealed, err := h.seal(in.Token)
	if err != nil {
		fail(w, 400, err)
		return
	}
	lock := WorkspaceLock(h.DataDir, wsctx.From(r.Context()))
	lock.Lock()
	defer lock.Unlock()
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck
	createdDir := ""
	committed := false
	defer func() {
		if !committed && createdDir != "" {
			_ = os.RemoveAll(createdDir)
		}
	}()
	var id int64
	err = tx.QueryRowContext(r.Context(), `INSERT INTO repository(title,kind,url,branch) VALUES(?,?,?,?) RETURNING id`, in.Title, in.Kind, in.URL, in.Branch).Scan(&id)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if in.Kind == "local" {
		createdDir = h.directory(wsctx.From(r.Context()), id)
		if err = Init(r.Context(), h.directory(wsctx.From(r.Context()), id)); err == nil {
			_, err = Git(r.Context(), h.directory(wsctx.From(r.Context()), id), "symbolic-ref", "HEAD", "refs/heads/"+in.Branch)
		}
		if err != nil {
			fail(w, 500, err)
			return
		}
	}
	if in.Kind == "local" {
		used, e := Bytes(filepath.Dir(createdDir))
		if e != nil {
			fail(w, 500, e)
			return
		}
		if used > h.quota() {
			fail(w, 413, fmt.Errorf("превышена квота репозиториев пространства"))
			return
		}
	}
	if sealed != "" {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO repository_token(repository_id,value) VALUES(?,?)`, id, sealed)
	}
	if err == nil && in.Owner != nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO repository_link(repository_id,owner_type,owner_id,path) VALUES(?,?,?,?)`, id, in.Owner.Kind, in.Owner.ID, in.Owner.Path)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, err)
		return
	}
	committed = true
	httpx.WriteJSON(w, 201, map[string]any{"id": id})
}
func (h *Handler) seal(token string) (string, error) {
	if token == "" {
		return "", nil
	}
	if len(token) > 4096 {
		return "", fmt.Errorf("секрет слишком длинный")
	}
	if h.Box == nil {
		return "", fmt.Errorf("хранилище секретов недоступно")
	}
	return h.Box.Seal(token)
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "change repositories") {
		return
	}
	repo, err := h.get(r)
	if err != nil {
		fail(w, 404, fmt.Errorf("репозиторий не найден"))
		return
	}
	var in struct {
		Title  *string `json:"title"`
		Branch *string `json:"branch"`
		Token  *string `json:"token"`
	}
	if !jsonBody(w, r, &in) {
		return
	}
	if in.Title != nil {
		repo.Title = strings.TrimSpace(*in.Title)
	}
	if in.Branch != nil {
		repo.Branch = *in.Branch
	}
	if repo.Title == "" || len(repo.Title) > 200 || !safeRef(repo.Branch) {
		fail(w, 400, fmt.Errorf("некорректные название или ветка"))
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 500, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck
	if in.Token != nil {
		if repo.Kind == "local" {
			fail(w, 400, fmt.Errorf("локальному репозиторию отдельный секрет не нужен"))
			return
		}
		var value string
		value, err = h.seal(*in.Token)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if value == "" {
			_, err = tx.ExecContext(r.Context(), `DELETE FROM repository_token WHERE repository_id=?`, repo.ID)
		} else {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO repository_token(repository_id,value) VALUES(?,?) ON CONFLICT(repository_id) DO UPDATE SET value=excluded.value`, repo.ID, value)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE repository SET title=?,branch=? WHERE id=?`, repo.Title, repo.Branch, repo.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}
func (h *Handler) owner(r *http.Request, l Link) error {
	table := ""
	switch l.Kind {
	case "quest":
		table = "quest"
	case "questline":
		table = "questline"
	case "step":
		table = "queststep"
	default:
		return fmt.Errorf("неизвестный владелец")
	}
	if !safePath(l.Path) {
		return fmt.Errorf("некорректный путь")
	}
	var exists bool
	err := h.DB.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id=?)", l.ID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("владелец не найден в пространстве")
	}
	return nil
}
func (h *Handler) link(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "link repositories") {
		return
	}
	repo, err := h.get(r)
	if err != nil {
		fail(w, 404, err)
		return
	}
	var l Link
	if !jsonBody(w, r, &l) {
		return
	}
	if err = h.owner(r, l); err != nil {
		fail(w, 400, err)
		return
	}
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO repository_link(repository_id,owner_type,owner_id,path) VALUES(?,?,?,?) ON CONFLICT(repository_id,owner_type,owner_id) DO UPDATE SET path=excluded.path`, repo.ID, l.Kind, l.ID, l.Path)
	if err != nil {
		fail(w, 500, err)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"ok": true})
}
func (h *Handler) unlink(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "unlink repositories") {
		return
	}
	repo, err := h.get(r)
	if err != nil {
		fail(w, 404, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	id, _ := strconv.ParseInt(r.URL.Query().Get("owner"), 10, 64)
	_, err = h.DB.ExecContext(r.Context(), `DELETE FROM repository_link WHERE repository_id=? AND owner_type=? AND owner_id=?`, repo.ID, kind, id)
	if err != nil {
		fail(w, 500, err)
		return
	}
	w.WriteHeader(204)
}
