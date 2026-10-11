// Package identityhttp is the HTTP face of the identity context (ADR-0001): sign-in and sessions, two-factor,
// API tokens, workspaces, members and invitations. The root (httpapi) owns the authentication middleware and
// hands this package what it needs from outside the context as plain functions.
package identityhttp

import (
	"context"
	"net/http"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/ratelimit"
)

// Handler serves the identity endpoints.
type Handler struct {
	Auth          *identity.Store
	AuthRequired  bool // reported by GET /api/auth/state; login is refused when false
	SecureCookies bool // force the Secure flag on the session cookie
	// Limiter is shared with the rest of the server (login, MFA, invitations and the assistant draw on one budget).
	Limiter *ratelimit.Limiter
	// Principal resolves who is calling, for the few endpoints that are public but behave differently when signed in.
	Principal func(*http.Request) (identity.Principal, bool)
	// OriginAllowed is the server's cross-origin policy for cookie-authenticated requests.
	OriginAllowed func(*http.Request) bool
	// Provision creates a workspace and, when copyFrom > 0, seeds it from that one: identity makes the workspace,
	// the data that fills it belongs to other contexts, so the root composes the steps.
	Provision func(ctx context.Context, userID int64, name string, copyFrom int64) (identity.Workspace, error)
	// Usage reports the attachment bytes the current workspace keeps and its quota (ok=false: unknown).
	Usage func(ctx context.Context) (used, quota int64, ok bool)

	mfa mfaTickets
}

// Register mounts every identity route.
func (h *Handler) Register(mux *http.ServeMux) {
	h.RegisterAuth(mux)
	h.RegisterMFA(mux)
	h.RegisterTokens(mux)
	h.RegisterWorkspaces(mux)
}
