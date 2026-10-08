package transform

import (
	"encoding/json"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolHistorySerializesEmptyArguments(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-6-thinking", "gemini-3.1-pro-low"} {
		for _, arguments := range []string{`{}`, `null`, ``, `invalid`, `{"limit":3}`} {
			t.Run(model+"/"+arguments, func(t *testing.T) {
				req := &openai.ChatCompletionRequest{
					Model: model,
					Messages: []openai.Message{
						{Role: "user", Content: "Get stats"},
						{Role: "assistant", ToolCalls: []openai.OpenAIToolCall{{
							ID: "call_1|signature", Type: "function",
							Function: openai.OpenAIFunctionCall{Name: "stats", Arguments: arguments},
						}}},
						{Role: "tool", ToolCallID: "call_1|signature", Content: ""},
					},
				}
				got, err := ToGeminiRequest(req, "test-project")
				require.NoError(t, err)
				encoded, err := json.Marshal(got)
				require.NoError(t, err)
				var wire struct {
					Request struct {
						Contents []struct {
							Parts []map[string]interface{} `json:"parts"`
						} `json:"contents"`
					} `json:"request"`
				}
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Len(t, wire.Request.Contents, 3)
				call := wire.Request.Contents[1].Parts[0]["functionCall"].(map[string]interface{})
				wantArgs := map[string]interface{}{}
				if arguments == `{"limit":3}` {
					wantArgs["limit"] = float64(3)
				}
				require.Equal(t, wantArgs, call["args"])
				require.Equal(t, "call_1", call["id"])
				require.Equal(t, "signature", wire.Request.Contents[1].Parts[0]["thoughtSignature"])
				result := wire.Request.Contents[2].Parts[0]["functionResponse"].(map[string]interface{})
				require.Equal(t, "call_1", result["id"])
				require.Equal(t, "stats", result["name"])
				require.Equal(t, map[string]interface{}{"output": ""}, result["response"])
			})
		}
	}
}

// Ensures tool responses (role: "tool") without an explicit name populate
// FunctionResponse.Name by resolving the assistant's tool_calls via tool_call_id.
// This prevents INVALID_ARGUMENT errors like:
// contents[n].parts[0].function_response.name: [REQUIRED_FIELD_MISSING]
func TestToolResponseNameResolutionViaToolCallID(t *testing.T) {
	req := &openai.ChatCompletionRequest{
		Model: "gemini-2.5-pro",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: "Please read poem.md",
			},
			{
				Role: "assistant",
				// Tool call emitted by the assistant. Note the ID "call_1".
				ToolCalls: []openai.OpenAIToolCall{
					{
						Index: 0,
						ID:    "call_1",
						Type:  "function",
						Function: openai.OpenAIFunctionCall{
							Name:      "read",
							Arguments: `{"file_path":"poem.md"}`,
						},
					},
				},
			},
			{
				// Tool result coming back. Name intentionally empty to force resolution via tool_call_id.
				Role:       "tool",
				ToolCallID: "call_1",
				Name:       "",
				Content:    "Once upon a midnight dreary...",
			},
		},
		Stream: true,
	}

	got, err := ToGeminiRequest(req, "test-project")
	require.NoError(t, err, "ToGeminiRequest should not error for tool response resolution")

	// We expect three contents turns (user, model, user[tool-response])
	require.Len(t, got.Request.Contents, 3, "expected 3 content turns (user, assistant, tool-response)")

	toolRespTurn := got.Request.Contents[2]
	// The transform maps "tool" role to "user" turn with FunctionResponse part
	assert.Equal(t, "user", toolRespTurn.Role, "tool responses are encoded as a user turn with functionResponse part")
	require.Len(t, toolRespTurn.Parts, 1, "expected a single part carrying the functionResponse")

	part := toolRespTurn.Parts[0]
	require.NotNil(t, part.FunctionResponse, "expected functionResponse to be populated")
	assert.Equal(t, "read", part.FunctionResponse.Name, "functionResponse.name must be resolved from tool_call_id")
	assert.Equal(t, "call_1", part.FunctionResponse.ID, "functionResponse.id must match tool_call_id")

	// Ensure the tool output was captured (we package it under a response map with 'output')
	require.NotNil(t, part.FunctionResponse.Response, "functionResponse.response should be non-nil")
	if out, ok := part.FunctionResponse.Response["output"].(string); ok {
		assert.Contains(t, out, "midnight dreary", "tool output should contain tool content")
	} else {
		t.Fatalf("functionResponse.response.output missing or not a string: %#v", part.FunctionResponse.Response)
	}
}

func TestToolResponsesInsertedBeforeAssistantText(t *testing.T) {
	req := &openai.ChatCompletionRequest{
		Model: "gemini-2.5-pro",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: "Read README",
			},
			{
				Role: "assistant",
				ToolCalls: []openai.OpenAIToolCall{
					{
						Index: 0,
						ID:    "call_1",
						Type:  "function",
						Function: openai.OpenAIFunctionCall{
							Name:      "read",
							Arguments: `{"file_path":"README.md"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				ToolCallID: "call_1",
				Content:    "README contents",
			},
			{
				Role:    "assistant",
				Content: "All done",
			},
		},
		Stream: true,
	}

	got, err := ToGeminiRequest(req, "test-project")
	require.NoError(t, err, "ToGeminiRequest should succeed")
	require.Len(t, got.Request.Contents, 4, "expected four content turns")

	assert.Equal(t, "user", got.Request.Contents[0].Role)
	assert.Equal(t, "model", got.Request.Contents[1].Role)
	assert.Equal(t, "user", got.Request.Contents[2].Role)
	assert.Equal(t, "model", got.Request.Contents[3].Role)

	toolResp := got.Request.Contents[2]
	require.Len(t, toolResp.Parts, 1)
	require.NotNil(t, toolResp.Parts[0].FunctionResponse)
	assert.Equal(t, "read", toolResp.Parts[0].FunctionResponse.Name)
	assert.Equal(t, "call_1", toolResp.Parts[0].FunctionResponse.ID)

	finalMsg := got.Request.Contents[3]
	require.Len(t, finalMsg.Parts, 1)
	assert.Equal(t, "All done", finalMsg.Parts[0].Text)
}

func TestConvertToolsToGeminiToolsSupportsBuiltIns(t *testing.T) {
	req := &openai.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []openai.Message{
			{Role: "user", Content: "What is the weather?"},
		},
		Tools: []openai.Tool{
			{Type: "web_search"},
			{Type: "url_context"},
			{Type: "function", Function: openai.Function{Name: "code_execution", Parameters: map[string]interface{}{}}},
			{
				Type: "function",
				Function: openai.Function{
					Name: "get_weather",
				},
			},
			{
				Type: "function",
				Function: openai.Function{
					Name:       "web_search",
					Parameters: map[string]interface{}{"type": "object"},
				},
			},
		},
	}

	got, err := ToGeminiRequest(req, "test-project")
	require.NoError(t, err)
	require.Len(t, got.Request.Tools, 4)
	require.Len(t, got.Request.Tools[0].FunctionDeclarations, 2)
	assert.Equal(t, "get_weather", got.Request.Tools[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "web_search", got.Request.Tools[0].FunctionDeclarations[1].Name)
	assert.NotNil(t, got.Request.Tools[1].GoogleSearch)
	assert.NotNil(t, got.Request.Tools[2].URLContext)
	assert.NotNil(t, got.Request.Tools[3].CodeExecution)
}
