package httpapi

import (
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/repository"
	"net/http"
)

func (s *Server) repositoryHTTP() *repository.Handler {
	return &repository.Handler{DB: s.Store.DB, DataDir: s.DataDir, Box: s.Secrets, Egress: s.Egress, RemoteEnabled: s.Store.Execution.EgressEnabled, RequireOwner: func(w http.ResponseWriter, r *http.Request, action string) bool {
		if wsctx.UserFrom(r.Context()) == wsctx.ActorAutomation {
			writeErr(w, 403, "only a workspace owner may "+action)
			return false
		}
		return s.identityHTTP().RequireOwner(w, r, action)
	}}
}
