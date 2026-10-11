package httpapi

import (
	"context"

	"github.com/quests-inc/quests/go/internal/identity"
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
	ws, fresh, err := st.Identity.EnsureUserWorkspace(ctx, userID, username)
	if err != nil || !fresh {
		return ws, err
	}
	if err := st.CopyCategories(ctx, identity.DefaultWorkspaceID, ws.ID); err != nil {
		return identity.Workspace{}, err
	}
	return ws, nil
}
