package identityhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
)

func TestWorkspaceAppearanceOwnerOnly(t *testing.T) {
	st := &identity.Store{DB: pgtest.New(t)}
	user, err := st.CreateUser("owner", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddWorkspaceMember(t.Context(), 1, user.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Auth: st}
	mux := http.NewServeMux()
	h.RegisterWorkspaces(mux)
	patch := func(body string) int {
		req := httptest.NewRequest(http.MethodPatch, "/api/workspaces/1", strings.NewReader(body))
		req = req.WithContext(identity.WithPrincipal(req.Context(), identity.Principal{User: user, Via: "session"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	if code := patch(`{"color":"#6a7ab8","icon":"map"}`); code != 200 {
		t.Fatal(code)
	}
	if code := patch(`{"name":"changed","color":"invalid"}`); code != 422 {
		t.Fatal(code)
	}
	if code := patch(`{}`); code != 422 {
		t.Fatal(code)
	}
	if err := st.AddWorkspaceMember(t.Context(), 1, user.ID, "member"); err != nil {
		t.Fatal(err)
	}
	if code := patch(`{"color":"#7a9e3a"}`); code != 403 {
		t.Fatal(code)
	}
}

func TestCreateWorkspaceWithAppearance(t *testing.T) {
	st := &identity.Store{DB: pgtest.New(t)}
	user, err := st.CreateUser("creator", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	h := &Handler{Auth: st, Provision: func(ctx context.Context, userID int64, name string, copyFrom int64) (identity.Workspace, error) {
		calls++
		return st.CreateWorkspace(ctx, userID, name)
	}}
	mux := http.NewServeMux()
	h.RegisterWorkspaces(mux)
	create := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces", strings.NewReader(body))
		req = req.WithContext(identity.WithPrincipal(req.Context(), identity.Principal{User: user, Via: "session"}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w.Code
	}
	if code := create(`{"name":"New","color":"invalid"}`); code != 422 || calls != 0 {
		t.Fatalf("%d calls %d", code, calls)
	}
	if code := create(`{"name":"New","color":"#6a7ab8","icon":"map"}`); code != 201 {
		t.Fatal(code)
	}
	rows, err := st.UserWorkspaces(t.Context(), user.ID)
	if err != nil || len(rows) != 1 || rows[0].Color != "#6a7ab8" || rows[0].Icon != "map" || rows[0].Role != "owner" {
		t.Fatalf("%+v %v", rows, err)
	}
}
