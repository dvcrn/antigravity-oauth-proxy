package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
)

type googleAuthTestStore struct {
	session []byte
	creds   *credentials.OAuthCredentials
}

func (s *googleAuthTestStore) GetCredentials() (*credentials.OAuthCredentials, error) {
	if s.creds == nil {
		return nil, errors.New("no credentials")
	}
	copy := *s.creds
	return &copy, nil
}

func (s *googleAuthTestStore) SaveCredentials(creds *credentials.OAuthCredentials) error {
	copy := *creds
	s.creds = &copy
	return nil
}

func (s *googleAuthTestStore) RefreshToken() error {
	return nil
}

func (s *googleAuthTestStore) Name() string {
	return "test"
}

func (s *googleAuthTestStore) LoadGoogleAuthSession() ([]byte, error) {
	return append([]byte(nil), s.session...), nil
}

func (s *googleAuthTestStore) SaveGoogleAuthSession(session []byte) error {
	s.session = append([]byte(nil), session...)
	return nil
}

func (s *googleAuthTestStore) CompleteGoogleAuth(creds *credentials.OAuthCredentials) error {
	if err := s.SaveCredentials(creds); err != nil {
		return err
	}
	return s.SaveGoogleAuthSession([]byte(`{"status":"authenticated"}`))
}

type googleAuthHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f googleAuthHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGoogleAuthStart(t *testing.T) {
	t.Parallel()

	store := &googleAuthTestStore{}
	flow := newGoogleAuth(store, googleAuthHTTPClientFunc(nil))
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	flow.now = func() time.Time { return now }

	status, err := flow.Start()
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.Status != "pending" {
		t.Fatalf("Start() status = %q", status.Status)
	}
	var session googleAuthSession
	if err := json.Unmarshal(store.session, &session); err != nil {
		t.Fatalf("decode stored session: %v", err)
	}
	parsed, err := url.Parse(status.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	challenge := sha256.Sum256([]byte(session.Verifier))
	query := parsed.Query()
	if parsed.Host != "accounts.google.com" || parsed.Path != "/o/oauth2/v2/auth" {
		t.Errorf("authorization URL = %s", parsed)
	}
	if query.Get("state") != session.State || query.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Errorf("authorization query = %v", query)
	}
	if query.Get("access_type") != "offline" || query.Get("prompt") != "consent" {
		t.Errorf("authorization query = %v", query)
	}
	if status.ExpiresAt != now.Add(googleAuthLifetime).Format(time.RFC3339Nano) {
		t.Errorf("ExpiresAt = %q", status.ExpiresAt)
	}
}

func TestGoogleAuthComplete(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	store := &googleAuthTestStore{}
	flow := newGoogleAuth(store, googleAuthHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse request: %v", err)
		}
		if form.Get("code") != "authorization-code" || form.Get("code_verifier") != "verifier" || form.Get("redirect_uri") != credentials.OAuthRedirectURI {
			t.Errorf("token form = %v", form)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"access_token":"access-token","refresh_token":"refresh-token","expires_in":3600,"token_type":"Bearer","scope":"scope","id_token":"id-token"}`,
			)),
			Header: make(http.Header),
		}, nil
	}))
	flow.now = func() time.Time { return now }
	store.session = []byte(`{"status":"pending","state":"state","verifier":"verifier","expiresAt":` + jsonNumber(now.Add(googleAuthLifetime).UnixMilli()) + `}`)

	status, err := flow.Complete(t.Context(), "http://localhost:51121/oauth-callback?code=authorization-code&state=state")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if status.Status != "authenticated" {
		t.Errorf("Complete() status = %q", status.Status)
	}
	if store.creds == nil || store.creds.AccessToken != "access-token" || store.creds.RefreshToken != "refresh-token" || store.creds.IDToken != "id-token" {
		t.Errorf("stored credentials = %+v", store.creds)
	}
	if store.creds.ExpiryDate != now.Add(time.Hour).UnixMilli() {
		t.Errorf("ExpiryDate = %d", store.creds.ExpiryDate)
	}
}

func TestGoogleAuthCompleteRejectsInvalidState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "missing", input: "authorization-code", wantErr: "missing OAuth state"},
		{name: "mismatch", input: "http://localhost:51121/oauth-callback?code=test&state=wrong", wantErr: "state mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &googleAuthTestStore{
				session: []byte(`{"status":"pending","state":"expected","verifier":"verifier","expiresAt":` + jsonNumber(now.Add(googleAuthLifetime).UnixMilli()) + `}`),
			}
			flow := newGoogleAuth(store, googleAuthHTTPClientFunc(func(_ *http.Request) (*http.Response, error) {
				t.Fatal("HTTP client called for invalid state")
				return nil, nil
			}))
			flow.now = func() time.Time { return now }

			_, err := flow.Complete(t.Context(), tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Complete() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseGoogleAuthorizationInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantCode  string
		wantState string
	}{
		{name: "redirect URL", input: "http://localhost:51121/oauth-callback?code=abc&state=def", wantCode: "abc", wantState: "def"},
		{name: "code and state", input: "abc#def", wantCode: "abc", wantState: "def"},
		{name: "query", input: "code=abc&state=def", wantCode: "abc", wantState: "def"},
		{name: "code", input: "abc", wantCode: "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, state := parseGoogleAuthorizationInput(tt.input)
			if code != tt.wantCode || state != tt.wantState {
				t.Errorf("parseGoogleAuthorizationInput(%q) = %q, %q", tt.input, code, state)
			}
		})
	}
}

func jsonNumber(value int64) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
