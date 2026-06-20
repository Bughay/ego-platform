// internal/auth/middleware.go
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/Bughay/egolifter/internal/shared/lib"
)

// contextKey is an unexported type to prevent context key collisions.
type contextKey string

const claimsContextKey contextKey = "jwt_claims"

// Middleware returns an http.Handler middleware that validates the JWT on every request.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			lib.WriteError(w, http.StatusUnauthorized, "authorization header is required")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			lib.WriteError(w, http.StatusUnauthorized, "authorization header format must be: Bearer {token}")
			return
		}

		claims, err := m.ValidateAccess(parts[1])
		if err != nil {
			lib.WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}

		// Inject the validated claims into the request context
		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CookieMiddleware for page routes
func (m *Manager) CookieMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			lib.WriteError(w, http.StatusUnauthorized, "authentication cookie required")
			return
		}

		claims, err := m.ValidateRefresh(cookie.Value)
		if err != nil {
			// Clear bad cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "refresh_token",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   false,
				SameSite: http.SameSiteStrictMode,
			})
			lib.WriteError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ClaimsFromContext retrieves the validated JWT claims from the request context.
// Returns nil if not present (i.e., called on an unprotected route).
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsContextKey).(*Claims)
	return claims
}

// RequireRole returns a middleware that enforces a specific user role.
func (m *Manager) RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || claims.Role != role {
			lib.WriteError(w, http.StatusForbidden, "you do not have permission to perform this action")
			return
		}
		next.ServeHTTP(w, r)
	})
}
