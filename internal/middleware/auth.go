package middleware

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"github.com/Ahmed20011994/anton/internal/httpx"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

const APIKeyHeader = "X-Anton-Key"

// RequireAPIKey compares the X-Anton-Key request header against the
// tenant's stored bcrypt hash (placed on the context by RequireTenant).
func RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := tenantctx.AuthHashFrom(r.Context())
		if hash == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "missing tenant context")
			return
		}
		key := r.Header.Get(APIKeyHeader)
		if key == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "missing api key")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)); err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid api key")
			return
		}
		next.ServeHTTP(w, r)
	})
}
