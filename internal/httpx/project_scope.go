package httpx

import "context"

type projectScopeKey struct{}
type sessionIDKey struct{}

// nil means global access; an empty non-nil slice means no project access.
func WithProjectScope(ctx context.Context, ids []int64) context.Context {
	return context.WithValue(ctx, projectScopeKey{}, ids)
}
func ProjectScope(ctx context.Context) []int64 {
	ids, _ := ctx.Value(projectScopeKey{}).([]int64)
	return ids
}
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}
func SessionID(ctx context.Context) string { id, _ := ctx.Value(sessionIDKey{}).(string); return id }
