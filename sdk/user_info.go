package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// UserResolver resolves an external application's bearer token into the user
// information expected by GoChat.
type UserResolver interface {
	ResolveUser(ctx context.Context, token string) (ExternalUser, error)
}

// UserResolverFunc adapts a function into a UserResolver.
type UserResolverFunc func(context.Context, string) (ExternalUser, error)

// ResolveUser implements UserResolver.
func (f UserResolverFunc) ResolveUser(ctx context.Context, token string) (ExternalUser, error) {
	return f(ctx, token)
}

// NewUserInfoHandler creates an http.Handler for the external user information
// endpoint configured in GoChat's auth.external_user_api.
//
// The resolver receives the bearer token without the "Bearer " prefix. Any
// resolver error is returned as HTTP 401 because GoChat treats this endpoint
// as an authentication decision point.
func NewUserInfoHandler(resolver UserResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if resolver == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user resolver is not configured"})
			return
		}

		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		user, err := resolver.ResolveUser(r.Context(), token)
		if err != nil || strings.TrimSpace(user.ID) == "" || strings.TrimSpace(user.Name) == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid external token"})
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
}

func bearerToken(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < len("Bearer ") || !strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(value[len("Bearer "):])
	return token, token != ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
