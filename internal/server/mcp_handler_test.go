package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider satisfies credentials.CredentialsProvider without touching disk
// or the network. The MCP protocol tests never reach an upstream call.
type stubProvider struct{}

func (stubProvider) GetCredentials() (*credentials.OAuthCredentials, error) {
	return &credentials.OAuthCredentials{AccessToken: "test-token"}, nil
}
func (stubProvider) SaveCredentials(*credentials.OAuthCredentials) error { return nil }
func (stubProvider) RefreshToken() error                                 { return nil }
func (stubProvider) Name() string                                        { return "stub" }

const testAdminKey = "test-admin-key"

func newMCPTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("ADMIN_API_KEY", testAdminKey)
	return NewServer(stubProvider{}, "test-project")
}

// postMCP sends one JSON-RPC message to /mcp and returns the decoded response.
func postMCP(t *testing.T, srv *Server, payload map[string]interface{}, authorized bool) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authorized {
		req.Header.Set("Authorization", "Bearer "+testAdminKey)
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return rec, nil
	}

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &decoded))
	return rec, decoded
}

func TestMCPEndpointRequiresAdminKey(t *testing.T) {
	srv := newMCPTestServer(t)

	rec, _ := postMCP(t, srv, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}, false)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMCPInitialize(t *testing.T) {
	srv := newMCPTestServer(t)

	rec, resp := postMCP(t, srv, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test-client", "version": "0.0.1"},
		},
	}, true)

	require.Equal(t, http.StatusOK, rec.Code)
	result, ok := resp["result"].(map[string]interface{})
	require.True(t, ok, "expected a result object, got %v", resp)

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, mcpServerName, serverInfo["name"])
	assert.Equal(t, mcpServerVersion, serverInfo["version"])
}

func TestMCPToolsList(t *testing.T) {
	srv := newMCPTestServer(t)

	rec, resp := postMCP(t, srv, map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}, true)

	require.Equal(t, http.StatusOK, rec.Code)
	result, ok := resp["result"].(map[string]interface{})
	require.True(t, ok, "expected a result object, got %v", resp)

	rawTools, ok := result["tools"].([]interface{})
	require.True(t, ok)

	tools := make(map[string]map[string]interface{}, len(rawTools))
	for _, rawTool := range rawTools {
		tool, ok := rawTool.(map[string]interface{})
		require.True(t, ok)
		name, ok := tool["name"].(string)
		require.True(t, ok)
		tools[name] = tool
	}

	require.Contains(t, tools, "ask_gemini")
	require.Contains(t, tools, "ask_gemini_models")

	// Both descriptions must tell the caller the answers come from Antigravity.
	for name, tool := range tools {
		description, ok := tool["description"].(string)
		require.True(t, ok, "tool %s has no description", name)
		assert.Contains(t, description, "Antigravity (agy) CLI", "tool %s", name)
	}

	schema, ok := tools["ask_gemini"]["inputSchema"].(map[string]interface{})
	require.True(t, ok)
	assert.ElementsMatch(t, []interface{}{"model", "prompt"}, schema["required"])
}

func TestMCPAskGeminiRejectsBlankInput(t *testing.T) {
	srv := newMCPTestServer(t)

	testCases := []struct {
		name        string
		args        map[string]interface{}
		wantMessage string
	}{
		{
			name:        "blank prompt",
			args:        map[string]interface{}{"model": "gemini-3.1-pro-high", "prompt": "   "},
			wantMessage: "prompt is required",
		},
		{
			name:        "blank model",
			args:        map[string]interface{}{"model": "  ", "prompt": "hello"},
			wantMessage: "model is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec, resp := postMCP(t, srv, map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params":  map[string]interface{}{"name": "ask_gemini", "arguments": tc.args},
			}, true)

			require.Equal(t, http.StatusOK, rec.Code)
			result, ok := resp["result"].(map[string]interface{})
			require.True(t, ok, "expected a result object, got %v", resp)

			// Tool-level failures come back as isError, not a JSON-RPC error.
			assert.Equal(t, true, result["isError"])
			assert.Contains(t, mcpResultText(t, result), tc.wantMessage)
		})
	}
}

func mcpResultText(t *testing.T, result map[string]interface{}) string {
	t.Helper()

	content, ok := result["content"].([]interface{})
	require.True(t, ok)

	var text string
	for _, rawPart := range content {
		part, ok := rawPart.(map[string]interface{})
		require.True(t, ok)
		if partText, ok := part["text"].(string); ok {
			text += partText
		}
	}
	return text
}

func TestExtractGeminiText(t *testing.T) {
	testCases := []struct {
		name     string
		response map[string]interface{}
		want     string
	}{
		{
			name:     "no candidates",
			response: map[string]interface{}{},
			want:     "",
		},
		{
			name: "joins text parts",
			response: map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"parts": []interface{}{
								map[string]interface{}{"text": "Hello, "},
								map[string]interface{}{"text": "world!"},
							},
						},
					},
				},
			},
			want: "Hello, world!",
		},
		{
			name: "skips thought parts",
			response: map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"parts": []interface{}{
								map[string]interface{}{"text": "thinking out loud", "thought": true},
								map[string]interface{}{"text": "the answer"},
							},
						},
					},
				},
			},
			want: "the answer",
		},
		{
			name: "ignores parts without text",
			response: map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"parts": []interface{}{
								map[string]interface{}{"functionCall": map[string]interface{}{"name": "noop"}},
								map[string]interface{}{"text": "answer"},
							},
						},
					},
				},
			},
			want: "answer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, extractGeminiText(tc.response))
		})
	}
}

// TestMCPAllowsNonLoopbackHost guards the DisableLocalhostProtection option.
// The SDK only applies DNS rebinding protection when the request carries a
// loopback http.LocalAddrContextKey, which a real listener sets and
// httptest.NewRequest does not - so this has to go through httptest.NewServer
// to exercise the check at all.
func TestMCPAllowsNonLoopbackHost(t *testing.T) {
	srv := newMCPTestServer(t)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	// A public hostname forwarded by a tunnel, which the rebinding check would
	// otherwise reject with 403 because the listener itself is loopback.
	req.Host = "proxy.example.com"

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", respBody)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &decoded))
	result, ok := decoded["result"].(map[string]interface{})
	require.True(t, ok, "expected a result object, got %v", decoded)
	require.NotEmpty(t, result["tools"])
}
