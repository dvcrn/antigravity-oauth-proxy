package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/openai"
)

func TestOpenAIChatCompletionsRejectsRemovedModels(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-6-thinking", "gpt-oss-120b-medium"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, stream), func(t *testing.T) {
				t.Parallel()
				body := fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"user","content":"hello"}]}`, model, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				rec := httptest.NewRecorder()
				(&Server{}).openAIChatCompletionsHandler(rec, req)
				if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), model) {
					t.Fatalf("expected removed-model error, got HTTP %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestNonStreamingAssistantMessageToolCalls(t *testing.T) {
	response := &antigravity.GenerateContentResponse{Response: map[string]interface{}{
		"candidates": []interface{}{map[string]interface{}{
			"content": map[string]interface{}{"parts": []interface{}{
				map[string]interface{}{"text": "thinking", "thought": true},
				map[string]interface{}{"functionCall": map[string]interface{}{"name": "search", "args": map[string]interface{}{"query": "notes"}}, "thoughtSignature": "signature"},
				map[string]interface{}{"functionCall": map[string]interface{}{"name": "lookup", "args": map[string]interface{}{"id": "123"}}},
			}},
		}},
	}}

	message, finishReason := nonStreamingAssistantMessage(response)
	if finishReason != "tool_calls" || message["content"] != nil || message["reasoning_content"] != "thinking" {
		t.Fatalf("unexpected message: %+v, finish reason: %s", message, finishReason)
	}
	calls, ok := message["tool_calls"].([]openai.OpenAIToolCall)
	if !ok || len(calls) != 2 {
		t.Fatalf("unexpected tool calls: %#v", message["tool_calls"])
	}
	if calls[0].Index != 0 || calls[1].Index != 1 || calls[0].Function.Name != "search" || calls[1].Function.Name != "lookup" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
	if !strings.HasSuffix(calls[0].ID, "|signature") || !strings.HasPrefix(calls[1].ID, "call_") {
		t.Fatalf("tool call IDs lost thought signature: %+v", calls)
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil || args["query"] != "notes" {
		t.Fatalf("unexpected arguments: %s (%v)", calls[0].Function.Arguments, err)
	}
}

func TestNonStreamingAssistantMessageText(t *testing.T) {
	response := &antigravity.GenerateContentResponse{Response: map[string]interface{}{
		"candidates": []interface{}{map[string]interface{}{
			"parts": []interface{}{
				map[string]interface{}{"text": "first"},
				map[string]interface{}{"text": "second"},
			},
		}},
	}}
	message, reason := nonStreamingAssistantMessage(response)
	if reason != "stop" || message["content"] != "first\nsecond" {
		t.Fatalf("unexpected message: %+v, reason: %s", message, reason)
	}
	if _, ok := message["tool_calls"]; ok {
		t.Fatal("plain text response included tool calls")
	}
}
