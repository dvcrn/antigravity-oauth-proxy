//go:build js && wasm

package credentials

import (
	"encoding/json"
	"fmt"

	serverhttp "github.com/dvcrn/antigravity-oauth-proxy/internal/http"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	"github.com/syumai/workers/cloudflare/kv"
)

const googleAuthSessionKey = "google-auth-session"

// CloudflareKVProvider implements CredentialsProvider using Cloudflare KV storage
type CloudflareKVProvider struct {
	kvStore    *kv.Namespace
	httpClient serverhttp.HTTPClient
}

// NewCloudflareKVProvider creates a new Cloudflare KV-based credentials provider
func NewCloudflareKVProvider() (*CloudflareKVProvider, error) {
	// In Cloudflare Workers, KV namespaces are accessed via bindings
	// The binding name is configured in wrangler.toml
	kvStore, err := kv.NewNamespace("ANTIGRAVITY_AUTH")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize KV namespace: %w", err)
	}

	return &CloudflareKVProvider{
		kvStore:    kvStore,
		httpClient: serverhttp.NewHTTPClient(),
	}, nil
}

// GetCredentials retrieves credentials from Cloudflare KV
func (c *CloudflareKVProvider) GetCredentials() (*OAuthCredentials, error) {
	// Get credentials JSON from KV
	credsJSON, err := c.kvStore.GetString("gemini_cli_oauth_credentials", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials from KV: %w", err)
	}

	if credsJSON == "" {
		return nil, fmt.Errorf("no credentials found in KV storage")
	}

	// Parse JSON
	var creds OAuthCredentials
	if err := json.Unmarshal([]byte(credsJSON), &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	return &creds, nil
}

// SaveCredentials saves credentials to Cloudflare KV
func (c *CloudflareKVProvider) SaveCredentials(creds *OAuthCredentials) error {
	// Marshal to JSON
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Store in KV
	if err := c.kvStore.PutString("gemini_cli_oauth_credentials", string(credsJSON), nil); err != nil {
		return fmt.Errorf("failed to store credentials in KV: %w", err)
	}

	logger.Get().Info().Msg("Saved credentials to Cloudflare KV")
	return nil
}

func (c *CloudflareKVProvider) LoadGoogleAuthSession() ([]byte, error) {
	session, err := c.kvStore.GetString(googleAuthSessionKey, nil)
	if err != nil {
		return nil, fmt.Errorf("get Google auth session: %w", err)
	}
	if session == "" || session == "<null>" {
		return nil, nil
	}
	return []byte(session), nil
}

func (c *CloudflareKVProvider) SaveGoogleAuthSession(session []byte) error {
	if err := c.kvStore.PutString(googleAuthSessionKey, string(session), nil); err != nil {
		return fmt.Errorf("save Google auth session: %w", err)
	}
	return nil
}

func (c *CloudflareKVProvider) CompleteGoogleAuth(creds *OAuthCredentials) error {
	if err := c.SaveCredentials(creds); err != nil {
		return err
	}
	return c.SaveGoogleAuthSession([]byte(`{"status":"authenticated"}`))
}

// RefreshToken refreshes the OAuth token using the refresh token
func (c *CloudflareKVProvider) RefreshToken() error {
	creds, err := c.GetCredentials()
	if err != nil {
		return fmt.Errorf("failed to get credentials for refresh: %w", err)
	}
	updated, err := refreshOAuthToken(c.httpClient, creds)
	if err != nil {
		return err
	}
	if err := c.SaveCredentials(updated); err != nil {
		return fmt.Errorf("save refreshed credentials: %w", err)
	}
	logger.Get().Info().Msg("Successfully refreshed OAuth token")
	return nil
}

// Name returns the provider name
func (c *CloudflareKVProvider) Name() string {
	return "CloudflareKVProvider"
}
