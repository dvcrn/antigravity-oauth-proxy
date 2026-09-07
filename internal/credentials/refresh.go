package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	serverhttp "github.com/dvcrn/antigravity-oauth-proxy/internal/http"
)

const (
	googleTokenURL      = "https://oauth2.googleapis.com/token"
	tokenRequestTimeout = 30 * time.Second
	tokenResponseLimit  = 1 << 20
)

func refreshOAuthToken(client serverhttp.HTTPClient, creds *OAuthCredentials) (*OAuthCredentials, error) {
	if creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	form := url.Values{
		"client_id":     {OAuthClientID},
		"client_secret": {OAuthClientSecret},
		"refresh_token": {creds.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), tokenRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token refresh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send token refresh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var refreshResp TokenRefreshResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, tokenResponseLimit)).Decode(&refreshResp); err != nil {
		return nil, fmt.Errorf("decode token refresh response: %w", err)
	}
	if refreshResp.AccessToken == "" || refreshResp.ExpiresIn <= 0 {
		return nil, fmt.Errorf("invalid token refresh response")
	}

	updated := *creds
	updated.AccessToken = refreshResp.AccessToken
	updated.ExpiryDate = time.Now().Add(time.Duration(refreshResp.ExpiresIn) * time.Second).UnixMilli()
	if refreshResp.TokenType != "" {
		updated.TokenType = refreshResp.TokenType
	}
	if refreshResp.Scope != "" {
		updated.Scope = refreshResp.Scope
	}
	return &updated, nil
}
