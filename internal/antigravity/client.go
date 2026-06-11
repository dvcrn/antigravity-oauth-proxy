package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	serverhttp "github.com/dvcrn/antigravity-oauth-proxy/internal/http"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

type UpstreamError struct {
	StatusCode  int
	Body        []byte
	ContentType string
	Endpoint    string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "upstream error"
	}

	const maxPreview = 1024
	preview := string(e.Body)
	if len(preview) > maxPreview {
		preview = preview[:maxPreview] + "..."
	}

	if e.Endpoint != "" {
		return fmt.Sprintf("upstream %s returned status %d: %s", e.Endpoint, e.StatusCode, preview)
	}
	return fmt.Sprintf("upstream returned status %d: %s", e.StatusCode, preview)
}

// Client is a client for the Antigravity Cloud Code API.
type Client struct {
	httpClient serverhttp.HTTPClient
	provider   credentials.CredentialsProvider
}

// NewClient creates a new Antigravity API client.
func NewClient(provider credentials.CredentialsProvider) *Client {
	return &Client{
		httpClient: serverhttp.NewHTTPClient(),
		provider:   provider,
	}
}

func (c *Client) doRequest(ctx context.Context, method string, url string, body []byte, accept string) (*http.Response, error) {
	creds, err := c.provider.GetCredentials()
	if err != nil {
		return nil, fmt.Errorf("unable to get credentials: %w", err)
	}

	if creds.AccessToken == "" {
		return nil, fmt.Errorf("access token is empty")
	}

	resp, err := c.doRequestWithToken(ctx, method, url, body, accept, creds.AccessToken)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	resp.Body.Close()

	if err := c.provider.RefreshToken(); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	refreshedCreds, err := c.provider.GetCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to reload credentials after refresh: %w", err)
	}

	return c.doRequestWithToken(ctx, method, url, body, accept, refreshedCreds.AccessToken)
}

func (c *Client) doRequestWithToken(ctx context.Context, method string, url string, body []byte, accept string, token string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}

	ApplyHeaders(req.Header, token, accept)
	req.Host = req.URL.Host
	if strings.EqualFold(accept, "text/event-stream") {
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request execution error: %w", err)
	}

	return resp, nil
}

// LoadCodeAssist performs a request to the Cloud Code API to check if the credentials are valid.
func (c *Client) LoadCodeAssist() (*LoadCodeAssistResponse, error) {
	requestBody := LoadCodeAssistRequest{
		Metadata: Metadata{
			IdeType: "ANTIGRAVITY",
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("could not marshal request body: %w", err)
	}

	var lastErr error
	for _, endpoint := range Endpoints {
		url := fmt.Sprintf("%s/v1internal:loadCodeAssist", endpoint)
		resp, err := c.doRequest(context.Background(), "POST", url, bodyBytes, "application/json")
		if err != nil {
			lastErr = err
			logger.Get().Warn().Err(err).Str("endpoint", endpoint).Msg("loadCodeAssist request failed")
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("could not read response body: %w", err)
			logger.Get().Warn().Err(err).Str("endpoint", endpoint).Msg("loadCodeAssist response read failed")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("auth check failed with status %d: %s", resp.StatusCode, string(respBody))
			logger.Get().Warn().
				Int("status", resp.StatusCode).
				Str("endpoint", endpoint).
				Msg("loadCodeAssist returned non-OK status")
			continue
		}

		var result LoadCodeAssistResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("could not unmarshal response body: %w", err)
		}

		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("loadCodeAssist failed with no endpoints available")
}

// GenerateContent performs a request to the Cloud Code API to generate content.
func (c *Client) GenerateContent(req *GenerateContentRequest) (*GenerateContentResponse, error) {
	prepareAntigravityRequest(req)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("could not marshal request body: %w", err)
	}

	var lastErr error
	for _, endpoint := range Endpoints {
		url := fmt.Sprintf("%s/v1internal:generateContent", endpoint)
		resp, err := c.doRequest(context.Background(), "POST", url, bodyBytes, "application/json")
		if err != nil {
			lastErr = err
			logger.Get().Warn().Err(err).Str("endpoint", endpoint).Msg("generateContent request failed")
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("could not read response body: %w", err)
			logger.Get().Warn().Err(err).Str("endpoint", endpoint).Msg("generateContent response read failed")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = &UpstreamError{
				StatusCode:  resp.StatusCode,
				Body:        respBody,
				ContentType: resp.Header.Get("Content-Type"),
				Endpoint:    endpoint,
			}
			logger.Get().Warn().
				Int("status", resp.StatusCode).
				Str("endpoint", endpoint).
				Msg("generateContent returned non-OK status")
			continue
		}

		var result GenerateContentResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("could not unmarshal response body: %w", err)
		}

		return &result, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("generateContent failed with no endpoints available")
}

// StreamGenerateContent performs a streaming request and sends each raw SSE line to the provided channel.
// It does not transform or interpret SSE content; lines are forwarded as-is.
// The caller owns the lifecycle of the 'out' channel; this function will not close it.
func (c *Client) StreamGenerateContent(ctx context.Context, req *GenerateContentRequest, out chan<- string) error {
	prepareAntigravityRequest(req)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("could not marshal request body: %w", err)
	}

	var lastErr error
	for _, endpoint := range Endpoints {
		url := fmt.Sprintf("%s/v1internal:streamGenerateContent?alt=sse", endpoint)
		resp, err := c.doRequest(ctx, "POST", url, bodyBytes, "text/event-stream")
		if err != nil {
			lastErr = err
			logger.Get().Warn().Err(err).Str("endpoint", endpoint).Msg("streamGenerateContent request failed")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("streamGenerateContent failed with status %d and read error: %v", resp.StatusCode, readErr)
				logger.Get().Warn().Err(readErr).Str("endpoint", endpoint).Msg("streamGenerateContent response read failed")
				continue
			}

			const maxPreview = 1024
			rprev := string(respBody)
			if len(rprev) > maxPreview {
				rprev = rprev[:maxPreview] + "..."
			}
			qprev := string(bodyBytes)
			if len(qprev) > maxPreview {
				qprev = qprev[:maxPreview] + "..."
			}
			logger.Get().Error().
				Int("status", resp.StatusCode).
				Str("endpoint", endpoint).
				Int("response_body_len", len(respBody)).
				Str("response_body_preview", rprev).
				Int("request_body_len", len(bodyBytes)).
				Str("request_body_preview", qprev).
				Msg("Upstream error on streamGenerateContent")

			lastErr = &UpstreamError{
				StatusCode:  resp.StatusCode,
				Body:        respBody,
				ContentType: resp.Header.Get("Content-Type"),
				Endpoint:    endpoint,
			}
			continue
		}

		// Start a goroutine to stream lines to the provided channel.
		go func() {
			defer resp.Body.Close()
			defer close(out)

			scanner := bufio.NewScanner(resp.Body)
			// Increase the scanner buffer for large SSE events
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)

			for scanner.Scan() {
				out <- scanner.Text()
			}
			if err := scanner.Err(); err != nil {
				logger.Get().Warn().Err(err).Msg("Upstream SSE scanner error")
			}
		}()

		return nil
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("streamGenerateContent failed with no endpoints available")
}
