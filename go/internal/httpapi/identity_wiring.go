package httpapi

import (
	"context"

	"github.com/valirum/quests/go/internal/identity"
	identityhttp "github.com/valirum/quests/go/internal/identity/http"
)

// identityHTTP is the identity context's HTTP face, wired to this server (built on first use, so the fields it
// copies are those set when the server was assembled).
func (s *Server) identityHTTP() *identityhttp.Handler {
	s.identityOnce.Do(func() {
		auth := s.Auth
		if auth == nil && s.Store != nil {
			auth = s.Store.Identity()
		}
		s.idh = &identityhttp.Handler{
			Auth:          auth,
			AuthRequired:  s.AuthRequired,
			SecureCookies: s.SecureCookies,
			Limiter:       s.limiter(),
			Principal:     s.principal,
			OriginAllowed: s.originAllowed,
			Provision: func(ctx context.Context, userID int64, name string, copyFrom int64) (identity.Workspace, error) {
				return ProvisionWorkspace(ctx, s.Store, userID, name, copyFrom)
			},
			Usage: func(ctx context.Context) (int64, int64, bool) {
				used, err := s.Store.AttachmentUsage(ctx)
				return used, s.WorkspaceQuota, err == nil
			},
		}
	})
	return s.idh
}
