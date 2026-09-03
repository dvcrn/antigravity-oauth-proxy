package antigravity

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCredentialsProvider struct{}

func (stubCredentialsProvider) GetCredentials() (*credentials.OAuthCredentials, error) {
	return &credentials.OAuthCredentials{AccessToken: "test-token"}, nil
}
func (stubCredentialsProvider) SaveCredentials(*credentials.OAuthCredentials) error { return nil }
func (stubCredentialsProvider) RefreshToken() error                                 { return nil }
func (stubCredentialsProvider) Name() string                                        { return "stub" }

// stubHTTPClient serves a canned response per requested model and records the
// order in which models were asked for.
type stubHTTPClient struct {
	statusForModel map[string]int
	requestedModel []string
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	// The request body is the CloudCode wrapper, so the model is a top-level field.
	var wrapper struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	c.requestedModel = append(c.requestedModel, wrapper.Model)

	status, ok := c.statusForModel[wrapper.Model]
	if !ok {
		status = http.StatusNotFound
	}

	responseBody := `{"error":"not found"}`
	if status == http.StatusOK {
		responseBody = `{"response":{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}}`
	}

	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(responseBody))),
	}, nil
}

func newTestClient(stub *stubHTTPClient) *Client {
	return &Client{
		httpClient: stub,
		provider:   stubCredentialsProvider{},
	}
}

func TestGenerateContentReportsServedModel(t *testing.T) {
	stub := &stubHTTPClient{
		statusForModel: map[string]int{"gemini-3.1-pro-low": http.StatusOK},
	}

	resp, err := newTestClient(stub).GenerateContent(&GenerateContentRequest{
		Model:   "gemini-3.1-pro-low",
		Request: GeminiInternalRequest{Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hi"}}}}},
	})

	require.NoError(t, err)
	assert.Equal(t, "gemini-3.1-pro-low", resp.Model)
	assert.Equal(t, []string{"gemini-3.1-pro-low"}, stub.requestedModel)
}

func TestGenerateContentReportsTieredFallbackModel(t *testing.T) {
	// A 3.7-flash model that 404s must fall back to the tiered variant, and the
	// response has to name the model that actually served it.
	stub := &stubHTTPClient{
		statusForModel: map[string]int{
			"gemini-3.7-flash-high":   http.StatusNotFound,
			"gemini-3.7-flash-tiered": http.StatusOK,
		},
	}

	resp, err := newTestClient(stub).GenerateContent(&GenerateContentRequest{
		Model:   "gemini-3.7-flash-high",
		Request: GeminiInternalRequest{Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hi"}}}}},
	})

	require.NoError(t, err)
	assert.Equal(t, "gemini-3.7-flash-tiered", resp.Model)
	assert.Equal(t, []string{"gemini-3.7-flash-high", "gemini-3.7-flash-tiered"}, stub.requestedModel)
}

func TestGenerateContentReportsTieredFallbackModel38(t *testing.T) {
	stub := &stubHTTPClient{
		statusForModel: map[string]int{
			"gemini-3.8-flash-high":   http.StatusNotFound,
			"gemini-3.8-flash-tiered": http.StatusOK,
		},
	}

	resp, err := newTestClient(stub).GenerateContent(&GenerateContentRequest{
		Model:   "gemini-3.8-flash-high",
		Request: GeminiInternalRequest{Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hi"}}}}},
	})

	require.NoError(t, err)
	assert.Equal(t, "gemini-3.8-flash-tiered", resp.Model)
	assert.Equal(t, []string{"gemini-3.8-flash-high", "gemini-3.8-flash-tiered"}, stub.requestedModel)
}

func TestGenerateContentDoesNotFallBackForOtherModels(t *testing.T) {
	stub := &stubHTTPClient{
		statusForModel: map[string]int{"gemini-3.1-pro-low": http.StatusNotFound},
	}

	_, err := newTestClient(stub).GenerateContent(&GenerateContentRequest{
		Model:   "gemini-3.1-pro-low",
		Request: GeminiInternalRequest{Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hi"}}}}},
	})

	require.Error(t, err)
	assert.Equal(t, []string{"gemini-3.1-pro-low"}, stub.requestedModel)
}
