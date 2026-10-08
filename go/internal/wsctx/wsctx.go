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
