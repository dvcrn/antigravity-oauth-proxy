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

func TestOpenAIChatCompletionsBareGeminiModelRouting(t *testing.T) {
	for _, tc := range []struct {
		name            string
		requestedModel  string
		reasoningEffort string
		wantUpstream    string
	}{
		{
			name:            "known family routes by reasoning_effort",
			requestedModel:  "gemini-4-carbon",
			reasoningEffort: "high",
			wantUpstream:    "gemini-4-carbon-high",
		},
		{
			name:            "catalog family routes by reasoning_effort",
			requestedModel:  "gemini-4-neon",
			reasoningEffort: "low",
			wantUpstream:    "gemini-4-neon-low",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamModels []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "fetchAvailableModels") {
					_, _ = w.Write([]byte(`{"defaultAgentModelId":"gemini-3.8-flash-high","models":{
						"gemini-3.8-flash-high":{},
						"gemini-4-carbon-low":{},
						"gemini-4-carbon-medium":{},
						"gemini-4-carbon-high":{},
						"gemini-4-neon-low":{},
						"gemini-4-neon-medium":{}
					}}`))
					return
				}
				var reqBody struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Fatal(err)
				}
				upstreamModels = append(upstreamModels, reqBody.Model)
				_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}}`))
			}))
			defer upstream.Close()
			previousEndpoints := antigravity.Endpoints
			antigravity.Endpoints = []string{upstream.URL}
			t.Cleanup(func() { antigravity.Endpoints = previousEndpoints })
			srv := newMCPTestServer(t)

			payload := fmt.Sprintf(`{"model":%q,"reasoning_effort":%q,"messages":[{"role":"user","content":"hi"}]}`, tc.requestedModel, tc.reasoningEffort)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
			rec := httptest.NewRecorder()
			srv.openAIChatCompletionsHandler(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}

			var resp struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Model != tc.requestedModel {
				t.Fatalf("response model = %q, want %q", resp.Model, tc.requestedModel)
			}
			if len(upstreamModels) != 1 || upstreamModels[0] != tc.wantUpstream {
				t.Fatalf("upstream models = %v, want [%q]", upstreamModels, tc.wantUpstream)
			}
		})
	}
}
