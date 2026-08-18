package api

import (
	"net/http"
	"strings"

	"github.com/relentlessworks/feedkit/internal/model"
)

// Middleware provides auth and CORS helpers.

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type contextKey string

const workspaceKey contextKey = "workspace"

func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public endpoints
		path := r.URL.Path
		if path == "/help" || path == "/.well-known/agent.md" || path == "/mcp" || path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow auth endpoints without token
		if path == "/auth/request" || path == "/auth/verify" {
			next.ServeHTTP(w, r)
			return
		}
		// Allow public entry view endpoints
		if strings.HasPrefix(path, "/public/") {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, r, http.StatusUnauthorized, "missing auth token", "call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token")
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeError(w, r, http.StatusUnauthorized, "invalid auth header", "use: Authorization: Bearer <token>")
			return
		}
		ws, err := h.auth.ValidateToken(parts[1])
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "invalid token", "call POST /auth/request with email to get a new OTP, then POST /auth/verify")
			return
		}
		// Store workspace in context
		r = withWorkspace(r, ws)
		next.ServeHTTP(w, r)
	})
}

func withWorkspace(r *http.Request, ws *model.Workspace) *http.Request {
	ctx := r.Context()
	ctx = contextWithWorkspace(ctx, ws)
	return r.WithContext(ctx)
}

func getWorkspace(r *http.Request) *model.Workspace {
	return workspaceFromContext(r.Context())
}
