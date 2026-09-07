package antigravity

import (
	"sync"
	"testing"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
)

type refreshTestProvider struct {
	mu        sync.Mutex
	creds     credentials.OAuthCredentials
	refreshes int
}

func (p *refreshTestProvider) GetCredentials() (*credentials.OAuthCredentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copy := p.creds
	return &copy, nil
}

func (p *refreshTestProvider) SaveCredentials(creds *credentials.OAuthCredentials) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creds = *creds
	return nil
}

func (p *refreshTestProvider) RefreshToken() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshes++
	p.creds.AccessToken = "refreshed-token"
	p.creds.ExpiryDate = time.Now().Add(time.Hour).UnixMilli()
	return nil
}

func (p *refreshTestProvider) Name() string {
	return "refresh-test"
}

func TestGetValidCredentialsRefreshesExpiringToken(t *testing.T) {
	t.Parallel()

	provider := &refreshTestProvider{
		creds: credentials.OAuthCredentials{
			AccessToken:  "expiring-token",
			RefreshToken: "refresh-token",
			ExpiryDate:   time.Now().Add(time.Minute).UnixMilli(),
		},
	}
	client := NewClient(provider)

	creds, err := client.getValidCredentials()
	if err != nil {
		t.Fatalf("getValidCredentials() error = %v", err)
	}
	if creds.AccessToken != "refreshed-token" {
		t.Errorf("AccessToken = %q", creds.AccessToken)
	}
	if provider.refreshes != 1 {
		t.Errorf("refreshes = %d, want 1", provider.refreshes)
	}
}
