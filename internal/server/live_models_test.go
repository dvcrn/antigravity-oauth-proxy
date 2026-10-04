//go:build integration

package server_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/openai"
	"github.com/stretchr/testify/require"
)

func TestLiveModelToolRoundTrip(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("PROXY_TEST_URL"), "/")
	key := os.Getenv("PROXY_TEST_API_KEY")
	require.NotEmpty(t, baseURL, "set PROXY_TEST_URL to the proxy under test")
	require.NotEmpty(t, key, "set PROXY_TEST_API_KEY to its admin key")
	models := []string{"gemini-3.8-flash-low"}
	if configured := os.Getenv("PROXY_TEST_MODELS"); configured != "" {
		models = strings.Split(configured, ",")
	}
	client := &http.Client{Timeout: 3 * time.Minute}
	for _, model := range models {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				// Omit max_tokens to exercise Claude thinking defaults as well as tool history.
				req := map[string]interface{}{
					"model": model, "stream": stream,
					"messages": []openai.Message{{Role: "user", Content: "Call proxy_test_stats exactly once with no arguments. Do not answer until you have its result."}},
					"tools": []openai.Tool{{Type: "function", Function: openai.Function{
						Name: "proxy_test_stats", Description: "Returns the test status. Call with no arguments.",
						Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
					}}},
				}
				first, reason := liveCompletion(t, client, baseURL, key, req)
				require.Equal(t, "tool_calls", reason)
				require.Len(t, first.ToolCalls, 1)
				call := first.ToolCalls[0]
				require.Equal(t, "proxy_test_stats", call.Function.Name)
				require.NotEmpty(t, call.ID)
				require.JSONEq(t, `{}`, call.Function.Arguments)
				history := append(req["messages"].([]openai.Message), first,
					openai.Message{Role: "tool", ToolCallID: call.ID, Content: "PROXY_TOOL_OK"})
				req["messages"] = history
				second, reason := liveCompletion(t, client, baseURL, key, req)
				require.Equal(t, "stop", reason)
				require.Empty(t, second.ToolCalls)
				require.Contains(t, second.Content, "PROXY_TOOL_OK")
				// Replay the same call with empty output to check required tool-result content.
				history[len(history)-1].Content = ""
				req["messages"] = append(history, openai.Message{Role: "user", Content: "The tool returned no text. Reply with exactly EMPTY_TOOL_OK and do not call another tool."})
				third, reason := liveCompletion(t, client, baseURL, key, req)
				require.Equal(t, "stop", reason)
				require.Empty(t, third.ToolCalls)
				require.Contains(t, third.Content, "EMPTY_TOOL_OK")
			})
		}
	}
}

func liveCompletion(t *testing.T, client *http.Client, baseURL, key string, payload map[string]interface{}) (openai.Message, string) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(request)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		t.Fatalf("proxy returned HTTP %d: %s", resp.StatusCode, body)
	}
	if !payload["stream"].(bool) {
		var response openai.ChatCompletionResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
		require.Equal(t, payload["model"], response.Model, "model fallback must not hide a failure")
		require.Len(t, response.Choices, 1)
		require.Equal(t, "assistant", response.Choices[0].Message.Role)
		return response.Choices[0].Message, response.Choices[0].FinishReason
	}
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	message := openai.Message{Role: "assistant"}
	var content strings.Builder
	calls := map[int]*openai.OpenAIToolCall{}
	reason := ""
	done := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			continue
		}
		require.False(t, done, "received data after [DONE]")
		var chunk struct {
			Model   string          `json:"model"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content   string                  `json:"content"`
					ToolCalls []openai.OpenAIToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &chunk))
		require.Empty(t, chunk.Error, "stream returned an error")
		require.Equal(t, payload["model"], chunk.Model, "model fallback must not hide a failure")
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)
			if choice.FinishReason != "" {
				reason = choice.FinishReason
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := calls[delta.Index]
				if call == nil {
					call = &openai.OpenAIToolCall{Index: delta.Index}
					calls[delta.Index] = call
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Type != "" {
					call.Type = delta.Type
				}
				call.Function.Name += delta.Function.Name
				call.Function.Arguments += delta.Function.Arguments
			}
		}
	}
	require.NoError(t, scanner.Err())
	require.True(t, done, "stream did not terminate with [DONE]")
	require.NotEmpty(t, reason, "stream lacked finish_reason")
	message.Content = content.String()
	for i := 0; i < len(calls); i++ {
		require.NotNil(t, calls[i], "tool indexes must be contiguous")
		message.ToolCalls = append(message.ToolCalls, *calls[i])
	}
	return message, reason
}
