package identity

import "time"

const (
	// InviteTTL is how long an invitation link stays valid.
	InviteTTL = 7 * 24 * time.Hour
	// WorkspacePurgeAfter is how long a deleted workspace can still be restored.
	WorkspacePurgeAfter = 7 * 24 * time.Hour
)
