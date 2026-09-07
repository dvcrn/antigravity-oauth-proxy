package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/env"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

// adminMiddleware checks the admin key in the supported Gemini and OpenAI locations.
func (s *Server) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminKey, ok := env.Get("ADMIN_API_KEY")
		if !ok || adminKey == "" {
			logger.Get().Error().Msg("ADMIN_API_KEY environment variable not set")
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Admin API not configured", http.StatusInternalServerError)
			return
		}

		var providedToken string
		authHeader := r.Header.Get("Authorization")
		googApiKey := r.Header.Get("X-Goog-Api-Key")
		xAPIKey := r.Header.Get("X-API-Key")
		keyParam := r.URL.Query().Get("key")

		if authHeader != "" {
			// Expect "Bearer <token>" format, case-insensitive
			parts := strings.Fields(authHeader)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				logger.Get().Warn().Msgf("Invalid Authorization header format for protected endpoint: %s %s from %s",
					r.Method, r.URL.Path, r.RemoteAddr)
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
				return
			}
			providedToken = parts[1]
		} else if googApiKey != "" {
			// Use X-Goog-Api-Key header directly
			providedToken = googApiKey
		} else if xAPIKey != "" {
			providedToken = xAPIKey
		} else if keyParam != "" {
			// Gemini-compatible clients commonly send API keys in this parameter.
			providedToken = keyParam
		} else {
			logger.Get().Warn().Msgf("Missing API key for protected endpoint: %s %s from %s",
				r.Method, r.URL.Path, r.RemoteAddr)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		providedHash := sha256.Sum256([]byte(providedToken))
		expectedHash := sha256.Sum256([]byte(adminKey))
		if subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) != 1 {
			logger.Get().Warn().Msgf("Invalid API key for protected endpoint: %s %s from %s",
				r.Method, r.URL.Path, r.RemoteAddr)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Admin authorized
		logger.Get().Info().Msgf("Protected request authorized: %s %s from %s",
			r.Method, r.URL.Path, r.RemoteAddr)

		next(w, r)
	}
}
