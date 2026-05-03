package middleware

import (
	"errors"
	"net/http"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

// RequireTenant resolves the {slug} path parameter into a tenant scope and
// stashes both the scope and the tenant's auth hash on the request context.
// It MUST run before RequireAPIKey.
func RequireTenant(repo *repository.TenantRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := r.PathValue("slug")
			if slug == "" {
				httpx.WriteError(w, http.StatusBadRequest, "missing tenant slug")
				return
			}
			t, err := repo.GetBySlug(r.Context(), slug)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					httpx.WriteError(w, http.StatusNotFound, "tenant not found")
					return
				}
				httpx.WriteError(w, http.StatusInternalServerError, "tenant lookup failed")
				return
			}
			ctx := tenantctx.With(r.Context(), tenantctx.Scope{TenantID: t.ID, Slug: t.Slug})
			ctx = tenantctx.WithAuthHash(ctx, t.APIKeyHash)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
