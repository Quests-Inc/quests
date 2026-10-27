package httpapi

import (
	"context"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

// Flows that cross contexts are composed here, at the root: identity makes workspaces, other contexts
// own the data that fills them (ADR-0001).

// ProvisionWorkspace creates a workspace and, when copyFrom > 0, gives it that workspace's category set so the
// new space is usable at once.
func ProvisionWorkspace(ctx context.Context, st *store.Store, userID int64, name string, copyFrom int64) (identity.Workspace, error) {
	ws, err := st.Identity.CreateWorkspace(ctx, userID, name)
	if err != nil {
		return identity.Workspace{}, err
	}
	if copyFrom > 0 {
		if err := st.CopyCategories(ctx, copyFrom, ws.ID); err != nil {
			return identity.Workspace{}, err
		}
	}
	return ws, nil
}

// EnsureUserWorkspace gives a user without any workspace one; a brand-new personal workspace gets the default categories.
func EnsureUserWorkspace(ctx context.Context, st *store.Store, userID int64, username string) (identity.Workspace, error) {
	previous, err := st.Identity.UserWorkspaces(ctx, userID)
	if err != nil {
		return identity.Workspace{}, err
	}
	ws, fresh, err := st.Identity.EnsureUserWorkspace(ctx, userID, username)
	if err != nil || len(previous) > 0 {
		return ws, err
	}
	if fresh {
		if err := st.CopyCategories(ctx, identity.DefaultWorkspaceID, ws.ID); err != nil {
			return identity.Workspace{}, err
		}
	}
	wctx := wsctx.With(ctx, ws.ID)
	// A legacy default workspace may already contain user data: claiming it must not add examples.
	var count int
	if err := st.DB.QueryRowContext(wctx, `SELECT (SELECT count(*) FROM note)+(SELECT count(*) FROM quest)+(SELECT count(*) FROM script)`).Scan(&count); err != nil {
		return identity.Workspace{}, err
	}
	if count == 0 {
		if _, err := st.SeedStarter(wctx); err != nil {
			return identity.Workspace{}, err
		}
	}
	return ws, nil
}
