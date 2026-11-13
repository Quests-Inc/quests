package store

import "context"

type graphScopeKey struct{}

// WithGraphScope selects a layout's draft inside the current workspace.
func WithGraphScope(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, graphScopeKey{}, key)
}
func graphScope(ctx context.Context) string {
	key, _ := ctx.Value(graphScopeKey{}).(string)
	return key
}
