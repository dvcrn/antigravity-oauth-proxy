package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolUnmarshalAndMarshalBuiltInTools(t *testing.T) {
	tests := []struct {
		name           string
		inputJSON      string
		expectSearch   bool
		expectURLCtx   bool
		expectCodeExec bool
		expectFns      int
		expectedJSON   string
	}{
		{
			name:         "googleSearch camelCase",
			inputJSON:    `{"googleSearch": {}}`,
			expectSearch: true,
			expectedJSON: `{"googleSearch":{}}`,
		},
		{
			name:         "google_search snake_case",
			inputJSON:    `{"google_search": {}}`,
			expectSearch: true,
			expectedJSON: `{"googleSearch":{}}`,
		},
		{
			name:         "googleSearch with options",
			inputJSON:    `{"googleSearch": {"mode": "MODE_DYNAMIC"}}`,
			expectSearch: true,
			expectedJSON: `{"googleSearch":{"mode":"MODE_DYNAMIC"}}`,
		},
		{
			name:         "googleSearchRetrieval drops retrieval options",
			inputJSON:    `{"googleSearchRetrieval": {"dynamicRetrievalConfig": {"mode": "MODE_DYNAMIC"}}}`,
			expectSearch: true,
			expectedJSON: `{"googleSearch":{}}`,
		},
		{
			name:         "urlContext camelCase",
			inputJSON:    `{"urlContext": {}}`,
			expectURLCtx: true,
			expectedJSON: `{"urlContext":{}}`,
		},
		{
			name:         "url_context snake_case",
			inputJSON:    `{"url_context": {}}`,
			expectURLCtx: true,
			expectedJSON: `{"urlContext":{}}`,
		},
		{
			name:           "codeExecution camelCase",
			inputJSON:      `{"codeExecution": {}}`,
			expectCodeExec: true,
			expectedJSON:   `{"codeExecution":{}}`,
		},
		{
			name:           "code_execution snake_case",
			inputJSON:      `{"code_execution": {}}`,
			expectCodeExec: true,
			expectedJSON:   `{"codeExecution":{}}`,
		},
		{
			name:         "multiple built-in tools in single object",
			inputJSON:    `{"google_search": {}, "url_context": {}}`,
			expectSearch: true,
			expectURLCtx: true,
			expectedJSON: `{"googleSearch":{},"urlContext":{}}`,
		},
		{
			name:         "functionDeclarations camelCase",
			inputJSON:    `{"functionDeclarations": [{"name": "get_weather", "description": "Get weather"}]}`,
			expectFns:    1,
			expectedJSON: `{"functionDeclarations":[{"name":"get_weather","description":"Get weather"}]}`,
		},
		{
			name:         "function_declarations snake_case",
			inputJSON:    `{"function_declarations": [{"name": "get_weather", "description": "Get weather"}]}`,
			expectFns:    1,
			expectedJSON: `{"functionDeclarations":[{"name":"get_weather","description":"Get weather"}]}`,
		},
		{
			name:         "mixed function declarations and googleSearch",
			inputJSON:    `{"google_search": {}, "function_declarations": [{"name": "get_weather"}]}`,
			expectSearch: true,
			expectFns:    1,
			expectedJSON: `{"functionDeclarations":[{"name":"get_weather"}],"googleSearch":{}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tool Tool
			err := json.Unmarshal([]byte(tc.inputJSON), &tool)
			require.NoError(t, err)

			if tc.expectSearch {
				require.NotNil(t, tool.GoogleSearch, "GoogleSearch should not be nil")
			} else {
				require.Nil(t, tool.GoogleSearch)
			}

			if tc.expectURLCtx {
				require.NotNil(t, tool.URLContext, "URLContext should not be nil")
			} else {
				require.Nil(t, tool.URLContext)
			}

			if tc.expectCodeExec {
				require.NotNil(t, tool.CodeExecution, "CodeExecution should not be nil")
			} else {
				require.Nil(t, tool.CodeExecution)
			}

			require.Len(t, tool.FunctionDeclarations, tc.expectFns)

			marshaled, err := json.Marshal(tool)
			require.NoError(t, err)
			require.JSONEq(t, tc.expectedJSON, string(marshaled))
		})
	}
}

func TestGeminiInternalRequestPreservesBuiltInTools(t *testing.T) {
	tests := []struct {
		name         string
		inputJSON    string
		expectTools  int
		expectedJSON string
	}{
		{
			name: "array with snake_case google_search",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"google_search": {}}]
			}`,
			expectTools: 1,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}]
			}`,
		},
		{
			name: "array with camelCase googleSearch",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}]
			}`,
			expectTools: 1,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}]
			}`,
		},
		{
			name: "single object with google_search",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": {"google_search": {}}
			}`,
			expectTools: 1,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}]
			}`,
		},
		{
			name: "array with google_search and url_context",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"google_search": {}}, {"url_context": {}}]
			}`,
			expectTools: 2,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}, {"urlContext": {}}]
			}`,
		},
		{
			name: "array with custom function and googleSearch",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [
					{"googleSearch": {}},
					{"functionDeclarations": [{"name": "fetchData"}]}
				]
			}`,
			expectTools: 2,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [
					{"googleSearch": {}},
					{"functionDeclarations": [{"name": "fetchData", "parameters": {"type": "OBJECT"}}]}
				]
			}`,
		},
		{
			name: "raw tools format with type web_search",
			inputJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"type": "web_search"}]
			}`,
			expectTools: 1,
			expectedJSON: `{
				"contents": [{"role": "user", "parts": [{"text": "search"}]}],
				"tools": [{"googleSearch": {}}]
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req GeminiInternalRequest
			err := json.Unmarshal([]byte(tc.inputJSON), &req)
			require.NoError(t, err)
			require.Len(t, req.Tools, tc.expectTools)

			marshaled, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, tc.expectedJSON, string(marshaled))
		})
	}
}

func TestConvertRawToolsBuiltInMatching(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "web_search_20250305", "name": "web_search", "max_uses": 5},
		{"type": "code_execution"},
		{"name": "url_context"},
		{"name": "WebSearch", "input_schema": {"type": "object", "properties": {"query": {"type": "string"}}}},
		{"name": "Read", "input_schema": {"type": "object"}}
	]`)

	var req GeminiInternalRequest
	require.NoError(t, json.Unmarshal([]byte(`{"tools":`+string(raw)+`}`), &req))
	require.Len(t, req.Tools, 4)

	var names []string
	for _, fn := range req.Tools[0].FunctionDeclarations {
		names = append(names, fn.Name)
	}
	require.Equal(t, []string{"WebSearch", "Read"}, names)
	require.NotNil(t, req.Tools[1].GoogleSearch)
	require.NotNil(t, req.Tools[2].CodeExecution)
	require.NotNil(t, req.Tools[3].URLContext)
}

func TestPruneUnsupportedBuiltInTools(t *testing.T) {
	search := map[string]interface{}{}
	fn := []FunctionDeclaration{{Name: "read"}}

	tools, dropped := pruneUnsupportedBuiltInTools([]Tool{
		{GoogleSearch: search, URLContext: map[string]interface{}{}},
		{CodeExecution: map[string]interface{}{}},
	})
	require.Equal(t, 2, dropped)
	require.Equal(t, []Tool{{GoogleSearch: search}}, tools)

	tools, dropped = pruneUnsupportedBuiltInTools([]Tool{{FunctionDeclarations: fn}, {GoogleSearch: search}})
	require.Equal(t, 1, dropped)
	require.Equal(t, []Tool{{FunctionDeclarations: fn}}, tools)
}

func TestConvertRawToolsExtractsBuiltIns(t *testing.T) {
	raw := json.RawMessage(`[
		{"google_search": {}},
		{"url_context": {}},
		{"name": "custom_fn", "description": "test", "parameters": {"type": "object"}}
	]`)

	tools, ok := convertRawTools(raw)
	require.True(t, ok)
	require.Len(t, tools, 3)

	// First is function declaration
	require.Len(t, tools[0].FunctionDeclarations, 1)
	require.Equal(t, "custom_fn", tools[0].FunctionDeclarations[0].Name)

	// Built-in tools
	require.NotNil(t, tools[1].GoogleSearch)
	require.NotNil(t, tools[2].URLContext)
}
