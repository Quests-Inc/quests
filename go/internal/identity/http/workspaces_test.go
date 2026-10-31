package identityhttp

import (
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
