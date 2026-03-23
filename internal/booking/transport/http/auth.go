package http

import (
	"context"
	"net/http"
	"strings"

	bookingauth "appointment-service/internal/booking/auth"
	"appointment-service/internal/booking/domain"
)

type authContextKey struct{}

func AuthMiddleware(tokens *bookingauth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if tokens == nil {
				writeUnauthorized(w, "authentication is not configured")
				return
			}

			rawAuth := strings.TrimSpace(r.Header.Get("Authorization"))
			if rawAuth == "" {
				writeUnauthorized(w, "missing bearer token")
				return
			}
			parts := strings.SplitN(rawAuth, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				writeUnauthorized(w, "authorization header must be in format: Bearer <token>")
				return
			}

			identity, err := tokens.Parse(strings.TrimSpace(parts[1]))
			if err != nil {
				writeUnauthorized(w, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), authContextKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func identityFromContext(ctx context.Context) (domain.Identity, bool) {
	identity, ok := ctx.Value(authContextKey{}).(domain.Identity)
	return identity, ok
}

func isPublicRoute(path string) bool {
	return path == "/healthz" || path == "/auth/login" || path == "/auth/register"
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusUnauthorized, errorResponse{Error: apiError{Code: "unauthorized", Message: message}})
}
