package transform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/openai"
	"github.com/google/uuid"
)

// ToGeminiRequest converts an OpenAI chat completion request to a Gemini generateContent request.
func ToGeminiRequest(openAIReq *openai.ChatCompletionRequest, projectID string) (*antigravity.GenerateContentRequest, error) {
	var internalReq antigravity.GeminiInternalRequest

	// Handle messages and system instructions
	geminiContents, systemInstruction, err := convertMessagesToGeminiContents(openAIReq.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Handle tools
	geminiTools := convertToolsToGeminiTools(openAIReq.Tools)

	// Handle generation config
	var genCfg *antigravity.GeminiGenerationConfig
	if openAIReq.Temperature > 0 || openAIReq.MaxTokens > 0 {
		genCfg = &antigravity.GeminiGenerationConfig{
			Temperature:     openAIReq.Temperature,
			MaxOutputTokens: openAIReq.MaxTokens,
		}
	}

	internalReq = antigravity.GeminiInternalRequest{
		Contents:          geminiContents,
		SystemInstruction: systemInstruction,
		Tools:             geminiTools,
		GenerationConfig:  genCfg,
	}

	geminiReq := &antigravity.GenerateContentRequest{
		Model:   openAIReq.Model,
		Project: projectID,
		Request: internalReq,
	}

	return geminiReq, nil
}

// convertMessagesToGeminiContents converts OpenAI messages to Gemini's content format.
// It also extracts the system message as a separate systemInstruction.
func convertMessagesToGeminiContents(messages []openai.Message) (geminiContents []antigravity.Content, systemInstruction *antigravity.SystemInstruction, err error) {
	// Build tool_call_id -> function name map from assistant tool calls
	toolCallNameByID := map[string]string{}
	toolCallIDByName := map[string]string{}
	var pendingToolParts []antigravity.ContentPart
	for _, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					toolCallNameByID[tc.ID] = tc.Function.Name
				}
			}
		}
	}

	for _, msg := range messages {
		roleLower := strings.ToLower(msg.Role)
		isTool := roleLower == "tool"

		if !isTool && len(pendingToolParts) > 0 {
			geminiContents = append(geminiContents, antigravity.Content{
				Role:  "user",
				Parts: pendingToolParts,
			})
			pendingToolParts = nil
		}

		if roleLower == "system" {
			// Allow multiple system messages by concatenating their parts
			if systemInstruction == nil {
				systemInstruction = &antigravity.SystemInstruction{
					Role:  "system",
					Parts: []antigravity.ContentPart{},
				}
			}

			switch content := msg.Content.(type) {
			case string:
				if content != "" {
					systemInstruction.Parts = append(systemInstruction.Parts, antigravity.ContentPart{Text: content})
				}
			case []interface{}:
				// Support array content for system messages (e.g., [{"type":"text","text":"..."}])
				for _, part := range content {
					if p, ok := part.(map[string]interface{}); ok && p["type"] == "text" {
						if txt, ok2 := p["text"].(string); ok2 && txt != "" {
							systemInstruction.Parts = append(systemInstruction.Parts, antigravity.ContentPart{Text: txt})
						}
					}
				}
			default:
				// Ignore unsupported content types for system messages
			}
			continue // System message is not part of contents
		}

		var role string
		switch roleLower {
		case "user":
			role = "user"
		case "assistant":
			role = "model"
		case "tool":
			role = "user"
		default:
			role = "user" // Default to user
		}

		var parts []antigravity.ContentPart
		switch content := msg.Content.(type) {
		case string:
			if isTool {
				cleanID := msg.ToolCallID
				if idx := strings.Index(cleanID, "|"); idx != -1 {
					cleanID = cleanID[:idx]
				}
				resolvedName := msg.Name
				if resolvedName == "" && cleanID != "" {
					if n, ok := toolCallNameByID[cleanID]; ok {
						resolvedName = n
					}
				}
				if resolvedName == "" {
					return nil, nil, fmt.Errorf("tool response missing function name and unresolved tool_call_id")
				}

				// Log forwarding of tool response (string content) with preview
				preview := content
				if len(preview) > 300 {
					preview = preview[:300] + "..."
				}
				logger.Get().Info().
					Str("function", resolvedName).
					Str("tool_call_id", msg.ToolCallID).
					Int("response_len", len(content)).
					Str("response_preview", preview).
					Msg("Forwarding tool response to Gemini")

				resolvedID := strings.TrimSpace(cleanID)
				if resolvedID == "" && resolvedName != "" {
					if id, ok := toolCallIDByName[resolvedName]; ok {
						resolvedID = id
					}
				}

				resp := map[string]interface{}{"output": content}
				parts = append(parts, antigravity.ContentPart{
					FunctionResponse: &antigravity.FunctionResponse{
						ID:       resolvedID,
						Name:     resolvedName,
						Response: resp,
					},
				})
			} else {
				if content != "" {
					parts = append(parts, antigravity.ContentPart{Text: content})
				}
			}
		case []interface{}:
			if isTool {
				var buf strings.Builder
				for _, part := range content {
					if p, ok := part.(map[string]interface{}); ok && p["type"] == "text" {
						if txt, ok2 := p["text"].(string); ok2 && txt != "" {
							if buf.Len() > 0 {
								buf.WriteString("\n")
							}
							buf.WriteString(txt)
						}
					}
				}
				cleanID := msg.ToolCallID
				if idx := strings.Index(cleanID, "|"); idx != -1 {
					cleanID = cleanID[:idx]
				}
				resolvedName := msg.Name
				if resolvedName == "" && cleanID != "" {
					if n, ok := toolCallNameByID[cleanID]; ok {
						resolvedName = n
					}
				}
				if resolvedName == "" {
					return nil, nil, fmt.Errorf("tool response missing function name and unresolved tool_call_id")
				}

				// Log forwarding of tool response (aggregated text parts) with preview
				full := buf.String()
				preview := full
				if len(preview) > 300 {
					preview = preview[:300] + "..."
				}
				logger.Get().Info().
					Str("function", resolvedName).
					Str("tool_call_id", msg.ToolCallID).
					Int("response_len", len(full)).
					Str("response_preview", preview).
					Msg("Forwarding tool response to Gemini")

				resolvedID := strings.TrimSpace(cleanID)
				if resolvedID == "" && resolvedName != "" {
					if id, ok := toolCallIDByName[resolvedName]; ok {
						resolvedID = id
					}
				}

				resp := map[string]interface{}{"output": full}
				parts = append(parts, antigravity.ContentPart{
					FunctionResponse: &antigravity.FunctionResponse{
						ID:       resolvedID,
						Name:     resolvedName,
						Response: resp,
					},
				})
			} else {
				for _, part := range content {
					if p, ok := part.(map[string]interface{}); ok && p["type"] == "text" {
						if txt, ok2 := p["text"].(string); ok2 {
							parts = append(parts, antigravity.ContentPart{Text: txt})
						}
					}
					// TODO: Handle other part types like images
				}
			}
		default:
			// Ignore unsupported content types for now
		}

		// Map assistant tool calls to functionCall parts
		if roleLower == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]interface{}{}
				}
				id := tc.ID
				if strings.TrimSpace(id) == "" {
					id = "toolu_" + uuid.NewString()
				}
				var thoughtSignature string
				if idx := strings.Index(id, "|"); idx != -1 {
					thoughtSignature = id[idx+1:]
					id = id[:idx]
				}

				if thoughtSignature != "" {
					logger.Get().Info().Str("signature", thoughtSignature).Msg("Restored thought_signature from client tool call ID")
				}

				if tc.Function.Name != "" {
					toolCallNameByID[id] = tc.Function.Name
					toolCallIDByName[tc.Function.Name] = id
				}
				parts = append(parts, antigravity.ContentPart{
					ThoughtSignature: thoughtSignature,
					FunctionCall: &antigravity.FunctionCall{
						ID:   id,
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
		}

		if isTool {
			if len(parts) > 0 {
				pendingToolParts = append(pendingToolParts, parts...)
			}
			continue
		}
		if len(parts) > 0 {
			geminiContents = append(geminiContents, antigravity.Content{
				Role:  role,
				Parts: parts,
			})
		}
	}

	if len(pendingToolParts) > 0 {
		geminiContents = append(geminiContents, antigravity.Content{
			Role:  "user",
			Parts: pendingToolParts,
		})
	}
	return geminiContents, systemInstruction, nil
}

func convertToolsToGeminiTools(tools []openai.Tool) []antigravity.Tool {
	if len(tools) == 0 {
		return nil
	}

	var fns []antigravity.FunctionDeclaration
	for _, t := range tools {
		if strings.ToLower(t.Type) != "function" {
			continue
		}

		var geminiSchema *antigravity.GeminiParameterSchema
		if m, ok := t.Function.Parameters.(map[string]interface{}); ok {
			geminiSchema = convertToGeminiSchema(m)
		}

		convertedFn := antigravity.FunctionDeclaration{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  geminiSchema,
		}

		// For specific tools, log the before and after transformation for debugging
		// if t.Function.Name == "TodoWrite" {
		// 	originalJSON, _ := json.Marshal(t.Function)
		// 	convertedJSON, _ := json.Marshal(convertedFn)
		// 	logger.Get().Info().
		// 		Str("tool_name", t.Function.Name).
		// 		RawJSON("original_schema", originalJSON).
		// 		RawJSON("converted_schema", convertedJSON).
		// 		Msg("Dumping tool schema conversion from OpenAI to Gemini")
		// }

		fns = append(fns, convertedFn)
	}

	if len(fns) == 0 {
		return nil
	}

	return []antigravity.Tool{
		{FunctionDeclarations: fns},
	}
}

// convertToGeminiSchema recursively converts a generic map representing a JSON schema
// into the strongly-typed GeminiParameterSchema struct, only mapping supported fields.
func convertToGeminiSchema(input map[string]interface{}) *antigravity.GeminiParameterSchema {
	return antigravity.ConvertSchema(input)
}
