package identityhttp

import "github.com/valirum/quests/go/internal/identity"

// Response shapes of the identity endpoints (the ones the web client needs types for).

// WorkspacesView is the account's workspaces and what the current one has stored (absent when the usage is unknown).
type WorkspacesView struct {
	Current          int64                `json:"current"`
	Workspaces       []identity.Workspace `json:"workspaces"`
	AttachmentsUsed  *int64               `json:"attachments_used,omitempty"`
	AttachmentsQuota *int64               `json:"attachments_quota,omitempty"`
}
