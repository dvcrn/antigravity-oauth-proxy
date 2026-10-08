package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPGoogleSearchRejectsInvalidInput(t *testing.T) {
	srv := newMCPTestServer(t)

	testCases := []struct {
		name        string
		args        map[string]interface{}
		wantMessage string
	}{
		{
			name:        "blank query",
			args:        map[string]interface{}{"query": "   "},
			wantMessage: "query is required",
		},
		{
			name:        "removed model",
			args:        map[string]interface{}{"query": "hello", "model": "claude-sonnet-4-6"},
			wantMessage: "model has been removed from this proxy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec, resp := postMCP(t, srv, map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params":  map[string]interface{}{"name": "google_search", "arguments": tc.args},
			}, true)

			require.Equal(t, http.StatusOK, rec.Code)
			result, ok := resp["result"].(map[string]interface{})
			require.True(t, ok, "expected a result object, got %v", resp)
			assert.Equal(t, true, result["isError"])
			assert.Contains(t, mcpResultText(t, result), tc.wantMessage)
		})
	}
}

func TestExtractGrounding(t *testing.T) {
	response := map[string]interface{}{
		"candidates": []interface{}{map[string]interface{}{
			"groundingMetadata": map[string]interface{}{
				"webSearchQueries": []interface{}{"go 1.25 release date", "go 1.25 release date"},
				"groundingChunks": []interface{}{
					map[string]interface{}{"web": map[string]interface{}{"uri": "https://a.example", "title": "a.example"}},
					map[string]interface{}{"web": map[string]interface{}{"uri": "https://a.example", "title": "a.example"}},
					map[string]interface{}{"web": map[string]interface{}{"uri": "https://b.example"}},
					map[string]interface{}{"retrievedContext": map[string]interface{}{"uri": "ignored"}},
				},
			},
		}},
	}

	sources, queries := extractGrounding(response)
	assert.Equal(t, []googleSearchSource{
		{Title: "a.example", URL: "https://a.example"},
		{URL: "https://b.example"},
	}, sources)
	assert.Equal(t, []string{"go 1.25 release date"}, queries)

	sources, queries = extractGrounding(map[string]interface{}{})
	assert.Empty(t, sources)
	assert.Empty(t, queries)
}

// redirectStub answers HEAD requests for known grounding redirects with a 302.
type redirectStub map[string]string

func (r redirectStub) Do(req *http.Request) (*http.Response, error) {
	target, ok := r[req.URL.String()]
	if !ok {
		return nil, errors.New("unreachable")
	}
	return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {target}}}, nil
}

func TestResolveGroundingRedirects(t *testing.T) {
	redirect := func(id string) string { return groundingRedirectPrefix + id }
	srv := newMCPTestServer(t)
	srv.redirectClient = redirectStub{
		redirect("a"): "https://go.dev/blog/go1.27",
		redirect("b"): "https://go.dev/blog/go1.27",
		redirect("c"): "https://go.dev/doc/go1.27",
	}

	got := srv.resolveGroundingRedirects(context.Background(), []googleSearchSource{
		{Title: "go.dev", URL: redirect("a")},
		{Title: "go.dev", URL: redirect("b")},
		{Title: "go.dev", URL: redirect("c")},
		{Title: "offline", URL: redirect("unknown")},
		{Title: "direct", URL: "https://example.com"},
	})

	assert.Equal(t, []googleSearchSource{
		{Title: "go.dev", URL: "https://go.dev/blog/go1.27"},
		{Title: "go.dev", URL: "https://go.dev/doc/go1.27"},
		{Title: "offline", URL: redirect("unknown")},
		{Title: "direct", URL: "https://example.com"},
	}, got)
}
