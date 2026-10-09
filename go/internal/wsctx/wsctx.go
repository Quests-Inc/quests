// Package wsctx carries the current workspace id on a context.
//
// The HTTP layer (or a background loop iterating workspaces) puts it there;
// the PostgreSQL driver wrapper turns it into the session setting that the
// row-level-security policies read, and the event hub uses it to route events.
// Zero means "no workspace": the database then shows nothing (fail closed).
package wsctx

import "context"

type key struct{}

// With returns ctx carrying workspace id.
func With(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, key{}, id)
}

// From returns the workspace id on ctx, or 0.
func From(ctx context.Context) int64 {
	id, _ := ctx.Value(key{}).(int64)
	return id
}

type userKey struct{}

// Actor values besides real user ids (> 0).
const (
	// ActorAutomation is the loopback token the assistant uses on a user's behalf:
	// it may not do what only workspace owners may.
	ActorAutomation int64 = -1
)

// WithUser records who is acting (a user id, or ActorAutomation). Background work has none.
func WithUser(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

// UserFrom returns the acting user id, or 0 for background / unauthenticated work.
func UserFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(userKey{}).(int64)
	return id
}
