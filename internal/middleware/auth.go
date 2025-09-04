package middleware

import (
	"cc-forwarder/config"
	"net/http"
	"strings"
)

type AuthMiddleware struct {
	config config.AuthConfig
}

func NewAuthMiddleware(cfg config.AuthConfig) *AuthMiddleware {
	return &AuthMiddleware{
		config: cfg,
	}
}

func (am *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !am.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Invalid authorization format. Expected 'Bearer <token>'", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token != am.config.Token {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// UpdateConfig updates the auth middleware configuration
func (am *AuthMiddleware) UpdateConfig(cfg config.AuthConfig) {
	am.config = cfg
}