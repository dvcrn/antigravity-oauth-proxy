package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	serverhttp "github.com/dvcrn/antigravity-oauth-proxy/internal/http"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	userInfoURL    = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

func DefaultConfig() Config {
	return Config{
		ClientID:     credentials.OAuthClientID,
		ClientSecret: credentials.OAuthClientSecret,
		RedirectURI:  credentials.OAuthRedirectURI,
		Scopes: []string{
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/cclog",
			"https://www.googleapis.com/auth/experimentsandconfigs",
		},
	}
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
	Scope        string
	IDToken      string
}

type UserInfo struct {
	Email string `json:"email"`
}

func AuthorizationURL(cfg Config, state string, pkceChallenge string) (string, error) {
	if cfg.ClientID == "" || cfg.RedirectURI == "" {
		return "", fmt.Errorf("missing client_id or redirect_uri")
	}
	if len(cfg.Scopes) == 0 {
		return "", fmt.Errorf("no scopes configured")
	}

	params := url.Values{}
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", cfg.RedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(cfg.Scopes, " "))
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("include_granted_scopes", "true")
	params.Set("state", state)
	params.Set("code_challenge", pkceChallenge)
	params.Set("code_challenge_method", "S256")

	return googleAuthURL + "?" + params.Encode(), nil
}

func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GeneratePKCEVerifier() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

type CallbackResult struct {
	Code  string
	State string
}

func WaitForCallback(ctx context.Context, redirectURI string) (CallbackResult, error) {
	port, path, err := parseRedirectURI(redirectURI)
	if err != nil {
		return CallbackResult{}, err
	}

	resultCh := make(chan CallbackResult, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{}
	mux := http.NewServeMux()
	srv.Handler = mux

	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			writeHTML(w, http.StatusBadRequest, "Authentication failed", "OAuth error: "+htmlEscape(errStr))
			sendErrOnce(errCh, fmt.Errorf("oauth error: %s", errStr))
			return
		}

		code := q.Get("code")
		state := q.Get("state")
		if code == "" {
			writeHTML(w, http.StatusBadRequest, "Authentication failed", "No authorization code received.")
			sendErrOnce(errCh, errors.New("no authorization code received"))
			return
		}

		writeHTML(w, http.StatusOK, "Authentication successful", "You can close this window and return to the terminal.")
		sendResultOnce(resultCh, CallbackResult{Code: code, State: state})
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return CallbackResult{}, err
	}
	defer ln.Close()

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			sendErrOnce(errCh, serveErr)
		}
	}()

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return CallbackResult{}, ctx.Err()
	case err := <-errCh:
		_ = srv.Shutdown(context.Background())
		return CallbackResult{}, err
	case res := <-resultCh:
		_ = srv.Shutdown(context.Background())
		return res, nil
	}
}

func ExchangeCode(ctx context.Context, cfg Config, code string, pkceVerifier string) (Tokens, error) {
	return ExchangeCodeWithClient(ctx, serverhttp.NewHTTPClient(), cfg, code, pkceVerifier)
}

func ExchangeCodeWithClient(ctx context.Context, client serverhttp.HTTPClient, cfg Config, code string, pkceVerifier string) (Tokens, error) {
	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("code", code)
	form.Set("code_verifier", pkceVerifier)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", cfg.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Tokens{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Tokens{}, err
	}
	if raw.AccessToken == "" || raw.ExpiresIn <= 0 {
		return Tokens{}, fmt.Errorf("invalid token exchange response")
	}

	return Tokens{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresIn:    raw.ExpiresIn,
		TokenType:    raw.TokenType,
		Scope:        raw.Scope,
		IDToken:      raw.IDToken,
	}, nil
}

func FetchUserInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	return fetchUserInfoWithClient(ctx, serverhttp.NewHTTPClient(), accessToken)
}

func fetchUserInfoWithClient(ctx context.Context, client serverhttp.HTTPClient, accessToken string) (UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return UserInfo{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return UserInfo{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, fmt.Errorf("userinfo failed with status %d", resp.StatusCode)
	}

	var ui UserInfo
	if err := json.Unmarshal(body, &ui); err != nil {
		return UserInfo{}, err
	}
	return ui, nil
}

func parseRedirectURI(redirectURI string) (port int, path string, err error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return 0, "", err
	}
	if u.Scheme != "http" {
		return 0, "", fmt.Errorf("redirect_uri must be http://")
	}
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return 0, "", fmt.Errorf("redirect_uri must be localhost")
	}
	p := u.Port()
	if p == "" {
		return 0, "", fmt.Errorf("redirect_uri must include an explicit port")
	}
	parsedPort, err := net.LookupPort("tcp", p)
	if err != nil {
		return 0, "", fmt.Errorf("invalid redirect_uri port: %w", err)
	}
	cbPath := u.EscapedPath()
	if cbPath == "" {
		cbPath = "/"
	}
	return parsedPort, cbPath, nil
}

func writeHTML(w http.ResponseWriter, status int, title string, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("<!doctype html><html><head><meta charset=\"utf-8\"><title>" + htmlEscape(title) + "</title></head><body style=\"font-family:system-ui;padding:40px;\"><h1>" + htmlEscape(title) + "</h1><p>" + body + "</p></body></html>"))
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

func sendResultOnce(ch chan<- CallbackResult, res CallbackResult) {
	select {
	case ch <- res:
	default:
	}
}

func sendErrOnce(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func DefaultTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}
