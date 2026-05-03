package tenantctx

import (
	"context"

	"github.com/google/uuid"
)

type Scope struct {
	TenantID uuid.UUID
	Slug     string
}

type ctxScopeKey struct{}
type ctxAuthHashKey struct{}

func With(ctx context.Context, s Scope) context.Context {
	return context.WithValue(ctx, ctxScopeKey{}, s)
}

func From(ctx context.Context) (Scope, bool) {
	s, ok := ctx.Value(ctxScopeKey{}).(Scope)
	return s, ok
}

// WithAuthHash stashes the tenant's bcrypt'd api_key_hash so the auth
// middleware can compare against the request header without re-querying.
func WithAuthHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, ctxAuthHashKey{}, hash)
}

func AuthHashFrom(ctx context.Context) string {
	h, _ := ctx.Value(ctxAuthHashKey{}).(string)
	return h
}
