package credentials

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type credentialsHTTPClientFunc func(*http.Request) (*http.Response, error)

func (f credentialsHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRefreshOAuthToken(t *testing.T) {
	t.Parallel()

	before := time.Now()
	client := credentialsHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != googleTokenURL {
			t.Errorf("request = %s %s", req.Method, req.URL)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse request: %v", err)
		}
		if form.Get("refresh_token") != "refresh-token" || form.Get("client_id") != OAuthClientID || form.Get("client_secret") != OAuthClientSecret {
			t.Errorf("unexpected refresh form: %v", form)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"access_token":"new-access-token","expires_in":3600,"token_type":"Bearer","scope":"new-scope"}`,
			)),
			Header: make(http.Header),
		}, nil
	})
	original := &OAuthCredentials{
		AccessToken:  "old-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "old-type",
		Scope:        "old-scope",
		IDToken:      "id-token",
	}

	updated, err := refreshOAuthToken(client, original)
	if err != nil {
		t.Fatalf("refreshOAuthToken() error = %v", err)
	}
	if updated.AccessToken != "new-access-token" || updated.RefreshToken != "refresh-token" || updated.TokenType != "Bearer" || updated.Scope != "new-scope" || updated.IDToken != "id-token" {
		t.Errorf("updated credentials = %+v", updated)
	}
	if updated.ExpiryDate < before.Add(time.Hour-time.Second).UnixMilli() || updated.ExpiryDate > time.Now().Add(time.Hour+time.Second).UnixMilli() {
		t.Errorf("ExpiryDate = %d", updated.ExpiryDate)
	}
	if original.AccessToken != "old-access-token" {
		t.Errorf("original credentials were mutated: %+v", original)
	}
}

func TestRefreshOAuthTokenRejectsIncompleteResponse(t *testing.T) {
	t.Parallel()

	client := credentialsHTTPClientFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"expires_in":3600}`)),
			Header:     make(http.Header),
		}, nil
	})
	_, err := refreshOAuthToken(client, &OAuthCredentials{RefreshToken: "refresh-token"})
	if err == nil || !strings.Contains(err.Error(), "invalid token refresh response") {
		t.Fatalf("refreshOAuthToken() error = %v", err)
	}
}
