package store

import "time"

// Server-side lifetimes. They live apart from the types so that the generated TypeScript types of
// the API (go/tygo.yaml) can leave them out: a time.Duration expression has no TypeScript form.
const (
	// InviteTTL is how long an invitation link stays valid.
	InviteTTL = 7 * 24 * time.Hour
	// RegistrationTTL is how long a fresh registration token stays valid.
	RegistrationTTL = 24 * time.Hour
	// WorkspacePurgeAfter is how long a deleted workspace can still be restored.
	WorkspacePurgeAfter = 7 * 24 * time.Hour
	// ReadDedupe is the window in which repeated opens of a note count once.
	ReadDedupe = 10 * time.Minute
)
