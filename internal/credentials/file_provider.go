package credentials

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/env"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

// FileProvider implements CredentialsProvider using file-based storage
type FileProvider struct {
	filePath   string
	httpClient *http.Client
}

// NewFileProvider creates a new file-based credentials provider
func NewFileProvider() (*FileProvider, error) {
	provider := &FileProvider{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Determine the file path
	if err := provider.determineFilePath(); err != nil {
		return nil, err
	}

	return provider, nil
}

// determineFilePath sets the file path based on environment variables or defaults
func (f *FileProvider) determineFilePath() error {
	// 1. Check for file path in environment variable
	if credsPath, ok := env.Get("CLOUDCODE_OAUTH_CREDS_PATH"); ok {
		f.filePath = credsPath
		return nil
	}

	// 2. Use default path: ~/.config/antigravity-oauth-proxy/oauth_creds.json
	// We intentionally prefer ~/.config for parity with other antigravity tools.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		f.filePath = resolveCredsPath(filepath.Join(homeDir, ".config"))
		return nil
	}

	// Fallback: best-effort platform config dir.
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get home or config directory: %w", err)
	}
	f.filePath = resolveCredsPath(configDir)
	return nil
}

// resolveCredsPath returns the credentials path under the given config base
// directory. If the new "antigravity-oauth-proxy" location does not yet exist
// but the pre-rename "antigravity-proxy" location does, the legacy credentials
// are migrated to the new location so existing installs keep working on upgrade.
func resolveCredsPath(configBase string) string {
	newPath := filepath.Join(configBase, "antigravity-oauth-proxy", "oauth_creds.json")
	oldPath := filepath.Join(configBase, "antigravity-proxy", "oauth_creds.json")
	migrateLegacyCreds(oldPath, newPath)
	return newPath
}

// migrateLegacyCreds copies credentials from the legacy path to the new path
// when the new path is absent but the legacy one exists. It is best-effort: any
// failure is logged and the new path is still returned by the caller, leaving
// GetCredentials to surface a clear error if no credentials can be loaded.
func migrateLegacyCreds(oldPath, newPath string) {
	// Skip if the new path already exists (or stat failed for another reason).
	if _, err := os.Stat(newPath); err == nil || !os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		// No legacy credentials to migrate (or unreadable) — nothing to do.
		return
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o700); err != nil {
		logger.Get().Warn().Err(err).Msg("failed to create directory for migrated credentials")
		return
	}
	if err := os.WriteFile(newPath, data, 0o600); err != nil {
		logger.Get().Warn().Err(err).Msg("failed to migrate credentials to new path")
		return
	}

	logger.Get().Info().Msgf("Migrated OAuth credentials from %s to %s", oldPath, newPath)
}

// GetCredentials retrieves credentials from file or environment
func (f *FileProvider) GetCredentials() (*OAuthCredentials, error) {
	// Try to load from file first
	if f.filePath != "" {
		data, err := os.ReadFile(f.filePath)
		if err == nil {
			creds := &OAuthCredentials{}
			if err := json.Unmarshal(data, creds); err != nil {
				return nil, fmt.Errorf("failed to parse credentials from file: %w", err)
			}
			return creds, nil
		}
		// If file doesn't exist, continue to check environment variable
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read credentials file: %w", err)
		}
	}

	// Fallback to raw JSON from environment variable
	if credsJSON, ok := env.Get("CLOUDCODE_OAUTH_CREDS"); ok {
		creds := &OAuthCredentials{}
		if err := json.Unmarshal([]byte(credsJSON), creds); err != nil {
			return nil, fmt.Errorf("failed to parse CLOUDCODE_OAUTH_CREDS: %w", err)
		}
		// When using environment variable, disable file writing
		f.filePath = ""
		return creds, nil
	}

	return nil, fmt.Errorf("OAuth credentials not found. Please set CLOUDCODE_OAUTH_CREDS_PATH, place oauth_creds.json in %s, or set CLOUDCODE_OAUTH_CREDS", f.filePath)
}

// SaveCredentials saves credentials to file if file path is set
func (f *FileProvider) SaveCredentials(creds *OAuthCredentials) error {
	if f.filePath == "" {
		// When using environment variable, we can't save
		logger.Get().Warn().Msg("Cannot save credentials when using CLOUDCODE_OAUTH_CREDS environment variable")
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(f.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Marshal credentials
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write to file
	if err := os.WriteFile(f.filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write credentials to %s: %w", f.filePath, err)
	}

	logger.Get().Info().Msgf("Saved credentials to %s", f.filePath)
	return nil
}

// RefreshToken refreshes the OAuth token using the refresh token
func (f *FileProvider) RefreshToken() error {
	creds, err := f.GetCredentials()
	if err != nil {
		return fmt.Errorf("failed to get credentials for refresh: %w", err)
	}

	if creds.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	// Prepare refresh request
	form := url.Values{}
	form.Add("client_id", OAuthClientID)
	form.Add("client_secret", OAuthClientSecret)
	form.Add("refresh_token", creds.RefreshToken)
	form.Add("grant_type", "refresh_token")

	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute refresh request
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var refreshResp TokenRefreshResponse
	if err := json.Unmarshal(body, &refreshResp); err != nil {
		return err
	}

	// Update credentials
	creds.AccessToken = refreshResp.AccessToken
	creds.ExpiryDate = time.Now().Add(time.Duration(refreshResp.ExpiresIn)*time.Second).Unix() * 1000
	creds.TokenType = refreshResp.TokenType

	// Update scope if provided in refresh response
	if refreshResp.Scope != "" {
		creds.Scope = refreshResp.Scope
	}

	// Save updated credentials
	if err := f.SaveCredentials(creds); err != nil {
		logger.Get().Warn().Err(err).Msg("failed to save refreshed credentials")
		// Don't fail the refresh if save fails
	}

	logger.Get().Info().Msg("Successfully refreshed OAuth token")
	return nil
}

// Name returns the provider name
func (f *FileProvider) Name() string {
	if f.filePath != "" {
		return fmt.Sprintf("FileProvider(%s)", f.filePath)
	}
	return "FileProvider(env)"
}
