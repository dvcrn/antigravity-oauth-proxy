package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/auth"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	serverhttp "github.com/dvcrn/antigravity-oauth-proxy/internal/http"
)

const (
	googleAuthLifetime = 15 * time.Minute
	googleAuthTimeout  = 30 * time.Second
)

type googleAuthInputError struct {
	message string
}

func (e *googleAuthInputError) Error() string {
	return e.message
}

type googleAuthUpstreamError struct {
	err error
}

func (e *googleAuthUpstreamError) Error() string {
	return e.err.Error()
}

func (e *googleAuthUpstreamError) Unwrap() error {
	return e.err
}

type GoogleAuthStore interface {
	credentials.CredentialsProvider
	LoadGoogleAuthSession() ([]byte, error)
	SaveGoogleAuthSession([]byte) error
	CompleteGoogleAuth(*credentials.OAuthCredentials) error
}

type GoogleAuth struct {
	store  GoogleAuthStore
	client serverhttp.HTTPClient
	config auth.Config
	now    func() time.Time
	mu     sync.Mutex
}

type googleAuthSession struct {
	Status    string `json:"status"`
	State     string `json:"state,omitempty"`
	Verifier  string `json:"verifier,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
}

type googleAuthStatus struct {
	Status           string `json:"status"`
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
}

func newGoogleAuth(store GoogleAuthStore, client serverhttp.HTTPClient) *GoogleAuth {
	return &GoogleAuth{
		store:  store,
		client: client,
		config: auth.DefaultConfig(),
		now:    time.Now,
	}
}

func (a *GoogleAuth) Start() (googleAuthStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	current, err := a.status()
	if err != nil {
		return googleAuthStatus{}, err
	}
	if current.Status == "pending" {
		return current, nil
	}

	state, err := auth.GenerateState()
	if err != nil {
		return googleAuthStatus{}, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, _, err := auth.GeneratePKCEVerifier()
	if err != nil {
		return googleAuthStatus{}, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	session := googleAuthSession{
		Status:    "pending",
		State:     state,
		Verifier:  verifier,
		ExpiresAt: a.now().Add(googleAuthLifetime).UnixMilli(),
	}
	if err := a.saveSession(session); err != nil {
		return googleAuthStatus{}, err
	}
	return a.view(session)
}

func (a *GoogleAuth) Status() (googleAuthStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status()
}

func (a *GoogleAuth) status() (googleAuthStatus, error) {
	session, ok, err := a.loadSession()
	if err != nil {
		return googleAuthStatus{}, err
	}
	if !ok {
		return googleAuthStatus{Status: "idle"}, nil
	}
	if session.Status == "pending" && a.now().UnixMilli() >= session.ExpiresAt {
		return a.finish("expired")
	}
	return a.view(session)
}

func (a *GoogleAuth) Complete(ctx context.Context, input string) (googleAuthStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	session, ok, err := a.loadSession()
	if err != nil {
		return googleAuthStatus{}, err
	}
	if !ok || session.Status != "pending" {
		return googleAuthStatus{}, &googleAuthInputError{message: "no pending Google authorization"}
	}
	if a.now().UnixMilli() >= session.ExpiresAt {
		return a.finish("expired")
	}

	code, state := parseGoogleAuthorizationInput(input)
	if code == "" {
		return googleAuthStatus{}, &googleAuthInputError{message: "missing authorization code"}
	}
	if state == "" {
		return googleAuthStatus{}, &googleAuthInputError{message: "missing OAuth state"}
	}
	if !constantTimeEqual(state, session.State) {
		return googleAuthStatus{}, &googleAuthInputError{message: "OAuth state mismatch"}
	}

	ctx, cancel := context.WithTimeout(ctx, googleAuthTimeout)
	defer cancel()
	tokens, err := auth.ExchangeCodeWithClient(ctx, a.client, a.config, code, session.Verifier)
	if err != nil {
		return googleAuthStatus{}, &googleAuthUpstreamError{err: err}
	}
	if tokens.AccessToken == "" || tokens.ExpiresIn <= 0 {
		return googleAuthStatus{}, &googleAuthUpstreamError{err: errors.New("invalid Google token exchange response")}
	}
	refreshToken := tokens.RefreshToken
	if refreshToken == "" {
		if current, currentErr := a.store.GetCredentials(); currentErr == nil {
			refreshToken = current.RefreshToken
		}
	}
	if refreshToken == "" {
		return googleAuthStatus{}, &googleAuthUpstreamError{err: errors.New("Google token exchange did not return a refresh token")}
	}

	creds := &credentials.OAuthCredentials{
		AccessToken:  tokens.AccessToken,
		RefreshToken: refreshToken,
		ExpiryDate:   a.now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UnixMilli(),
		TokenType:    tokens.TokenType,
		Scope:        tokens.Scope,
		IDToken:      tokens.IDToken,
	}
	if err := a.store.CompleteGoogleAuth(creds); err != nil {
		return googleAuthStatus{}, fmt.Errorf("store Google credentials: %w", err)
	}
	return googleAuthStatus{Status: "authenticated"}, nil
}

func (a *GoogleAuth) loadSession() (googleAuthSession, bool, error) {
	encoded, err := a.store.LoadGoogleAuthSession()
	if err != nil {
		return googleAuthSession{}, false, err
	}
	if len(encoded) == 0 {
		return googleAuthSession{}, false, nil
	}
	var session googleAuthSession
	if err := json.Unmarshal(encoded, &session); err != nil {
		return googleAuthSession{}, false, fmt.Errorf("invalid stored Google auth session: %w", err)
	}
	return session, true, nil
}

func (a *GoogleAuth) saveSession(session googleAuthSession) error {
	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return a.store.SaveGoogleAuthSession(encoded)
}

func (a *GoogleAuth) finish(status string) (googleAuthStatus, error) {
	session := googleAuthSession{Status: status}
	if err := a.saveSession(session); err != nil {
		return googleAuthStatus{}, err
	}
	return a.view(session)
}

func (a *GoogleAuth) view(session googleAuthSession) (googleAuthStatus, error) {
	if session.Status != "pending" {
		return googleAuthStatus{Status: session.Status}, nil
	}
	challengeBytes := sha256.Sum256([]byte(session.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	authorizationURL, err := auth.AuthorizationURL(a.config, session.State, challenge)
	if err != nil {
		return googleAuthStatus{}, fmt.Errorf("build Google authorization URL: %w", err)
	}
	return googleAuthStatus{
		Status:           "pending",
		AuthorizationURL: authorizationURL,
		ExpiresAt:        time.UnixMilli(session.ExpiresAt).UTC().Format(time.RFC3339Nano),
	}, nil
}

func parseGoogleAuthorizationInput(input string) (code, state string) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.ParseRequestURI(value); err == nil && parsed.IsAbs() {
		return parsed.Query().Get("code"), parsed.Query().Get("state")
	}
	if before, after, ok := strings.Cut(value, "#"); ok {
		return before, after
	}
	if strings.Contains(value, "code=") {
		params, err := url.ParseQuery(value)
		if err == nil {
			return params.Get("code"), params.Get("state")
		}
	}
	return value, ""
}

func constantTimeEqual(left, right string) bool {
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}
