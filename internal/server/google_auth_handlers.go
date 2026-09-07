package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

func (s *Server) tokensHandler(w http.ResponseWriter, r *http.Request) {
	if !s.googleAuthRequestAllowed(w, r, http.MethodPost) {
		return
	}
	if !requireJSONContentType(w, r) {
		return
	}
	body, ok := readLimitedRequestBody(w, r, 65536)
	if !ok {
		return
	}
	var input struct {
		AccessToken  string  `json:"accessToken"`
		RefreshToken *string `json:"refreshToken,omitempty"`
		ExpiresAt    int64   `json:"expiresAt"`
		TokenType    *string `json:"tokenType,omitempty"`
		Scope        *string `json:"scope,omitempty"`
		IDToken      *string `json:"idToken,omitempty"`
	}
	if err := decodeStrictJSON(body, &input); err != nil {
		writeAdminError(w, "Expected accessToken, expiresAt, and optional refreshToken, tokenType, scope, and idToken", http.StatusBadRequest)
		return
	}
	invalidOptional := emptyOptional(input.RefreshToken) || emptyOptional(input.TokenType) || emptyOptional(input.Scope) || emptyOptional(input.IDToken)
	if strings.TrimSpace(input.AccessToken) == "" || input.ExpiresAt <= 0 || invalidOptional {
		writeAdminError(w, "Expected accessToken, expiresAt, and optional refreshToken, tokenType, scope, and idToken", http.StatusBadRequest)
		return
	}

	creds := &credentials.OAuthCredentials{
		AccessToken: strings.TrimSpace(input.AccessToken),
		ExpiryDate:  input.ExpiresAt,
		TokenType:   "Bearer",
	}
	if current, err := s.provider.GetCredentials(); err == nil {
		creds.RefreshToken = current.RefreshToken
		creds.TokenType = current.TokenType
		creds.Scope = current.Scope
		creds.IDToken = current.IDToken
	}
	if input.RefreshToken != nil {
		creds.RefreshToken = strings.TrimSpace(*input.RefreshToken)
	}
	if creds.RefreshToken == "" {
		writeAdminError(w, "Expected refreshToken when no refresh token is stored", http.StatusBadRequest)
		return
	}
	if input.TokenType != nil {
		creds.TokenType = strings.TrimSpace(*input.TokenType)
	}
	if creds.TokenType == "" {
		creds.TokenType = "Bearer"
	}
	if input.Scope != nil {
		creds.Scope = strings.TrimSpace(*input.Scope)
	}
	if input.IDToken != nil {
		creds.IDToken = strings.TrimSpace(*input.IDToken)
	}
	if err := s.provider.SaveCredentials(creds); err != nil {
		logger.Get().Error().Err(err).Msg("Failed to store OAuth credentials")
		writeAdminError(w, "Failed to store credentials", http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, map[string]bool{"stored": true})
}

func (s *Server) tokenStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !s.googleAuthRequestAllowed(w, r, http.MethodGet) {
		return
	}
	creds, err := s.provider.GetCredentials()
	writeAdminJSON(w, map[string]bool{
		"configured": err == nil && creds != nil && creds.AccessToken != "",
	})
}

func (s *Server) googleAuthStartHandler(w http.ResponseWriter, r *http.Request) {
	if !s.googleAuthRequestAllowed(w, r, http.MethodPost) {
		return
	}
	if !discardLimitedRequestBody(w, r, 1024) {
		return
	}
	status, err := s.googleAuth.Start()
	s.writeGoogleAuthResponse(w, status, err)
}

func (s *Server) googleAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !s.googleAuthRequestAllowed(w, r, http.MethodGet, http.MethodPost) {
		return
	}
	if r.Method == http.MethodGet {
		status, err := s.googleAuth.Status()
		s.writeGoogleAuthResponse(w, status, err)
		return
	}
	if !requireJSONContentType(w, r) {
		return
	}
	body, ok := readLimitedRequestBody(w, r, 65536)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeStrictJSON(body, &input); err != nil || strings.TrimSpace(input.Code) == "" {
		writeAdminError(w, "Expected an authorization code or redirect URL in code", http.StatusBadRequest)
		return
	}
	status, err := s.googleAuth.Complete(r.Context(), input.Code)
	s.writeGoogleAuthResponse(w, status, err)
}

func (s *Server) googleAuthRequestAllowed(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	allowed := false
	for _, method := range methods {
		if r.Method == method {
			allowed = true
			break
		}
	}
	if !allowed {
		w.Header().Set("Allow", strings.Join(methods, ", "))
		writeAdminError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	scheme := r.URL.Scheme
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if origin != scheme+"://"+r.Host {
		writeAdminError(w, "Origin is not allowed", http.StatusForbidden)
		return false
	}
	return true
}

func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeAdminError(w, "Expected application/json", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func emptyOptional(value *string) bool {
	return value != nil && strings.TrimSpace(*value) == ""
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func discardLimitedRequestBody(w http.ResponseWriter, r *http.Request, limit int64) bool {
	_, ok := readLimitedRequestBody(w, r, limit)
	return ok
}

func readLimitedRequestBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	if r.Body == nil {
		return nil, true
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		writeAdminError(w, "Invalid request body", http.StatusBadRequest)
		return nil, false
	}
	if int64(len(body)) > limit {
		writeAdminError(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return body, true
}

func (s *Server) writeGoogleAuthResponse(w http.ResponseWriter, status googleAuthStatus, err error) {
	if err != nil {
		var inputErr *googleAuthInputError
		if errors.As(err, &inputErr) {
			writeAdminError(w, inputErr.Error(), http.StatusBadRequest)
			return
		}
		logger.Get().Error().Err(err).Msg("Google authorization request failed")
		var upstreamErr *googleAuthUpstreamError
		if errors.As(err, &upstreamErr) {
			writeAdminError(w, "Google authorization request failed", http.StatusBadGateway)
			return
		}
		writeAdminError(w, "Google authorization request failed", http.StatusInternalServerError)
		return
	}
	writeAdminJSON(w, status)
}

func writeAdminJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeAdminError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.Error(w, message, status)
}
