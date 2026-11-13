package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestRepositoryGitTokenAndOwner(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	s.DataDir = t.TempDir()
	s.Auth = s.Store.Identity
	s.AuthRequired = true
	user, err := s.Auth.CreateUser("gitowner", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := s.Auth.CreateWorkspace(context.Background(), user.ID, "Sources")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Auth.CreateAPIToken(user.ID, "git", ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	r := httptest.NewRequest("POST", "/api/repositories", strings.NewReader(`{"title":"Owned"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var made struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &made)
	path := "/api/repositories/" + strconv.FormatInt(made.ID, 10) + "/git/info/refs?service=git-upload-pack"
	named := "/git/" + ws.Slug + "/repo-" + strconv.FormatInt(made.ID, 10) + ".git/info/refs?service=git-upload-pack"
	for _, login := range []string{"", user.Username, "different-user"} {
		request := httptest.NewRequest("GET", named, nil)
		if login != "" {
			request.SetBasicAuth(login, token)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		want := 401
		if login == user.Username {
			want = 200
		}
		if response.Code != want {
			t.Fatalf("named Git login=%q status=%d %s", login, response.Code, response.Body.String())
		}
	}
	other, err := s.Auth.CreateWorkspace(context.Background(), user.ID, "Other")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", strings.Replace(named, ws.Slug, other.Slug, 1), nil)
	request.SetBasicAuth(user.Username, token)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("token escaped its workspace: %d", response.Code)
	}

	r = httptest.NewRequest("GET", path, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing git auth challenge")
	}
	r = httptest.NewRequest("GET", path, nil)
	r.SetBasicAuth(user.Username, token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("basic token =%d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", path, nil)
	r.SetBasicAuth("different-user", token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("token accepted under another login")
	}
	if _, err = s.Auth.DB.Exec(`UPDATE workspace_member SET role='member' WHERE user_id=? AND workspace_id=?`, user.ID, ws.ID); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("GET", strings.Replace(named, "git-upload-pack", "git-receive-pack", 1), nil)
	r.SetBasicAuth(user.Username, token)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("member push advertisement =%d", w.Code)
	}
}
