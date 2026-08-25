package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpServerName    = "ask-antigravity"
	mcpServerVersion = "0.1.0"
)

// askGeminiInput is the input for the ask_gemini tool.
type askGeminiInput struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// askGeminiOutput is the structured result of the ask_gemini tool. Model is the
// model that actually served the request, which can differ from the requested
// one via model resolution or the client's 404 fallback.
type askGeminiOutput struct {
	RequestedModel string `json:"requested_model"`
	Model          string `json:"model"`
	Text           string `json:"text"`
}

// askGeminiModelsInput is the (empty) input for the ask_gemini_models tool.
type askGeminiModelsInput struct{}

type askGeminiModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

type askGeminiModelsOutput struct {
	DefaultModel string           `json:"default_model,omitempty"`
	Models       []askGeminiModel `json:"models"`
}

// newMCPServer builds the MCP server exposed at /mcp. Tools call the Antigravity
// API in-process through the same client the Gemini and OpenAI endpoints use.
func (s *Server) newMCPServer() *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    mcpServerName,
		Version: mcpServerVersion,
	}, nil)

	addMCPTool(srv, &mcpsdk.Tool{
		Name: "ask_gemini",
		Description: "Ask a Gemini model a single self-contained question and get its answer back as text. " +
			"Requests are served by the Antigravity (agy) CLI backend using the local Antigravity OAuth " +
			"credentials, so no separate Gemini API key is involved. Each call is one-shot: there is no " +
			"conversation history, so put everything the model needs into the prompt. Call ask_gemini_models " +
			"first if you are unsure which model IDs are available.",
		InputSchema: mcpObjectSchema(map[string]any{
			"model": mcpStringSchema("Model ID to ask, e.g. gemini-3.1-pro-high. " +
				"Use ask_gemini_models to list the IDs the Antigravity (agy) CLI backend currently offers."),
			"prompt": mcpStringSchema("The full question or instruction to send to the model."),
		}, "model", "prompt"),
	}, s.mcpAskGemini)

	addMCPTool(srv, &mcpsdk.Tool{
		Name: "ask_gemini_models",
		Description: "List the model IDs that can be passed to ask_gemini. Models come from the Antigravity " +
			"(agy) CLI backend and reflect what the current Antigravity account is entitled to, so the list " +
			"can differ between accounts and change over time.",
		InputSchema: mcpObjectSchema(map[string]any{}),
	}, s.mcpAskGeminiModels)

	return srv
}

// mcpHandler returns the stateless streamable HTTP handler for /mcp. The server
// keeps no per-session state, so every request is served with a fresh session
// and plain JSON responses instead of an SSE stream.
func (s *Server) mcpHandler() http.HandlerFunc {
	mcpServer := s.newMCPServer()
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return mcpServer },
		&mcpsdk.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
	return handler.ServeHTTP
}

func (s *Server) mcpAskGemini(ctx context.Context, in askGeminiInput) (askGeminiOutput, error) {
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return askGeminiOutput{}, fmt.Errorf("prompt is required")
	}

	requestedModel := strings.TrimSpace(in.Model)
	if requestedModel == "" {
		return askGeminiOutput{}, fmt.Errorf("model is required; call ask_gemini_models to list available models")
	}

	request := antigravity.GeminiInternalRequest{
		Contents: []antigravity.Content{{
			Role:  "user",
			Parts: []antigravity.ContentPart{{Text: prompt}},
		}},
	}
	applyModelThinkingDefaults(requestedModel, &request)
	resolvedModel := resolveModelForThinking(requestedModel, request)

	logger.Get().Info().
		Str("requested_model", requestedModel).
		Str("model", resolvedModel).
		Int("prompt_len", len(prompt)).
		Msg("MCP ask_gemini request received")

	apiCallStart := time.Now()
	resp, err := s.antigravityClient.GenerateContent(&antigravity.GenerateContentRequest{
		Model:   resolvedModel,
		Project: s.projectID,
		Request: request,
	})
	if err != nil {
		logger.Get().Error().
			Err(err).
			Str("requested_model", requestedModel).
			Str("model", resolvedModel).
			Dur("api_call_duration", time.Since(apiCallStart)).
			Msg("MCP ask_gemini GenerateContent failed")
		return askGeminiOutput{}, fmt.Errorf("ask_gemini failed for model %q: %w", requestedModel, err)
	}

	// The client can fall back to another model after a 404, so report the model
	// that actually served the response rather than the one we asked for.
	servedModel := resp.Model
	if servedModel == "" {
		servedModel = resolvedModel
	}

	text := extractGeminiText(resp.Response)
	if text == "" {
		return askGeminiOutput{}, fmt.Errorf("model %q returned no text", servedModel)
	}

	logger.Get().Info().
		Str("requested_model", requestedModel).
		Str("model", servedModel).
		Str("resolved_model", resolvedModel).
		Int("text_len", len(text)).
		Dur("api_call_duration", time.Since(apiCallStart)).
		Msg("MCP ask_gemini completed")

	return askGeminiOutput{
		RequestedModel: requestedModel,
		Model:          servedModel,
		Text:           text,
	}, nil
}

func (s *Server) mcpAskGeminiModels(ctx context.Context, _ askGeminiModelsInput) (askGeminiModelsOutput, error) {
	data, err := s.antigravityClient.FetchAvailableModels(ctx)
	if err != nil {
		logger.Get().Error().Err(err).Msg("MCP ask_gemini_models failed to fetch available models")
		return askGeminiModelsOutput{}, fmt.Errorf("failed to fetch available models: %w", err)
	}

	models := make([]askGeminiModel, 0, len(data.Models))
	for modelID, modelData := range data.Models {
		if !isSupportedModel(modelID) {
			continue
		}
		models = append(models, askGeminiModel{
			ID:          modelID,
			DisplayName: modelData.DisplayName,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	return askGeminiModelsOutput{
		DefaultModel: data.DefaultAgentModelID,
		Models:       models,
	}, nil
}

// extractGeminiText pulls the answer text out of an unwrapped CloudCode response.
// Parts flagged as thoughts are skipped so callers get the answer, not the
// model's reasoning trace.
func extractGeminiText(response map[string]interface{}) string {
	candidates, ok := response["candidates"].([]interface{})
	if !ok {
		return ""
	}

	var b strings.Builder
	for _, candidate := range candidates {
		candidateMap, ok := candidate.(map[string]interface{})
		if !ok {
			continue
		}
		content, ok := candidateMap["content"].(map[string]interface{})
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]interface{})
		if !ok {
			continue
		}
		for _, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			if isThought, ok := partMap["thought"].(bool); ok && isThought {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				b.WriteString(text)
			}
		}
	}

	return b.String()
}

// addMCPTool registers a tool whose handler returns a structured result, which
// the SDK marshals into both the structured content and the text fallback.
func addMCPTool[In, Out any](srv *mcpsdk.Server, tool *mcpsdk.Tool, handler func(context.Context, In) (Out, error)) {
	mcpsdk.AddTool(srv, tool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input In) (*mcpsdk.CallToolResult, Out, error) {
		output, err := handler(ctx, input)
		return nil, output, err
	})
}

func mcpObjectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpStringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
