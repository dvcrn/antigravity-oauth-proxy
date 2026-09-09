package transform

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/openai"
)

func TestToGeminiRequestReasoningEffort(t *testing.T) {
	t.Run("reasoning_effort none for 3.8 flash", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:           "gemini-3.8-flash-tiered",
			ReasoningEffort: "none",
			Messages:        []openai.Message{{Role: "user", Content: "hi"}},
		}

		gemReq, err := ToGeminiRequest(req, "test-proj")
		if err != nil {
			t.Fatalf("ToGeminiRequest failed: %v", err)
		}
		if gemReq.Request.GenerationConfig == nil || gemReq.Request.GenerationConfig.ThinkingConfig == nil {
			t.Fatal("expected ThinkingConfig to be non-nil")
		}
		tc := gemReq.Request.GenerationConfig.ThinkingConfig
		if tc.ThinkingBudget == nil || *tc.ThinkingBudget != 0 {
			t.Fatalf("ThinkingBudget = %v, want 0", tc.ThinkingBudget)
		}
	})

	t.Run("reasoning_effort off for 3.5 flash lite", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:           "gemini-3.5-flash-lite",
			ReasoningEffort: "off",
			Messages:        []openai.Message{{Role: "user", Content: "hi"}},
		}

		gemReq, err := ToGeminiRequest(req, "test-proj")
		if err != nil {
			t.Fatalf("ToGeminiRequest failed: %v", err)
		}
		if gemReq.Request.GenerationConfig == nil || gemReq.Request.GenerationConfig.ThinkingConfig == nil {
			t.Fatal("expected ThinkingConfig to be non-nil")
		}
		tc := gemReq.Request.GenerationConfig.ThinkingConfig
		if tc.ThinkingLevel != "THINKING_LEVEL_UNSPECIFIED" {
			t.Fatalf("ThinkingLevel = %q, want THINKING_LEVEL_UNSPECIFIED", tc.ThinkingLevel)
		}
		if tc.ThinkingBudget != nil {
			t.Fatalf("ThinkingBudget = %v, want nil", tc.ThinkingBudget)
		}
	})

	t.Run("thinking_budget zero for 3.5 flash lite converts to UNSPECIFIED", func(t *testing.T) {
		zero := 0
		req := &openai.ChatCompletionRequest{
			Model:          "gemini-3.5-flash-lite",
			ThinkingBudget: &zero,
			Messages:       []openai.Message{{Role: "user", Content: "hi"}},
		}

		gemReq, err := ToGeminiRequest(req, "test-proj")
		if err != nil {
			t.Fatalf("ToGeminiRequest failed: %v", err)
		}
		if gemReq.Request.GenerationConfig == nil || gemReq.Request.GenerationConfig.ThinkingConfig == nil {
			t.Fatal("expected ThinkingConfig to be non-nil")
		}
		tc := gemReq.Request.GenerationConfig.ThinkingConfig
		if tc.ThinkingLevel != "THINKING_LEVEL_UNSPECIFIED" {
			t.Fatalf("ThinkingLevel = %q, want THINKING_LEVEL_UNSPECIFIED", tc.ThinkingLevel)
		}
		if tc.ThinkingBudget != nil {
			t.Fatalf("ThinkingBudget = %v, want nil", tc.ThinkingBudget)
		}
	})

	t.Run("invalid reasoning_effort returns error", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:           "gemini-3.8-flash-tiered",
			ReasoningEffort: "invalid-effort",
			Messages:        []openai.Message{{Role: "user", Content: "hi"}},
		}

		_, err := ToGeminiRequest(req, "test-proj")
		if err == nil {
			t.Fatal("expected error for invalid reasoning_effort")
		}
	})
}

func TestConvertToGeminiSchema(t *testing.T) {
	testCases := []struct {
		name           string
		inputSchema    map[string]interface{}
		expectedSchema *antigravity.GeminiParameterSchema
	}{
		{
			name: "Simple Schema",
			inputSchema: map[string]interface{}{
				"type":        "object",
				"description": "A simple object.",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "The name.",
					},
				},
				"required": []interface{}{"name"},
			},
			expectedSchema: &antigravity.GeminiParameterSchema{
				Type:        "OBJECT",
				Description: "A simple object.",
				Properties: map[string]*antigravity.GeminiParameterSchema{
					"name": {
						Type:        "STRING",
						Description: "The name.",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			name: "TodoWrite Schema with anyOf",
			inputSchema: map[string]interface{}{
				"description": "The updated todo list",
				"anyOf": []interface{}{
					map[string]interface{}{
						"type":     "array",
						"maxItems": 50.0,
						"items": map[string]interface{}{
							"type":     "object",
							"required": []interface{}{"content", "status"},
							"properties": map[string]interface{}{
								"content": map[string]interface{}{
									"type": "string",
								},
								"status": map[string]interface{}{
									"type": "string",
									"enum": []interface{}{"pending", "completed"},
								},
							},
						},
					},
					map[string]interface{}{
						"type": "string",
					},
				},
			},
			expectedSchema: &antigravity.GeminiParameterSchema{
				Type:        "ARRAY",
				Description: "The updated todo list",
				Items: &antigravity.GeminiParameterSchema{
					Type:     "OBJECT",
					Required: []string{"content", "status"},
					Properties: map[string]*antigravity.GeminiParameterSchema{
						"content": {
							Type: "STRING",
						},
						"status": {
							Type: "STRING",
							Enum: []string{"pending", "completed"},
						},
					},
				},
			},
		},
		{
			name: "Schema with oneOf",
			inputSchema: map[string]interface{}{
				"description": "A parameter that can be one of several types.",
				"oneOf": []interface{}{
					map[string]interface{}{
						"type": "string",
					},
					map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "number",
						},
					},
				},
			},
			expectedSchema: &antigravity.GeminiParameterSchema{
				Type:        "ARRAY",
				Description: "A parameter that can be one of several types.",
				Items: &antigravity.GeminiParameterSchema{
					Type: "NUMBER",
				},
			},
		},
		{
			name: "Schema with unsupported keywords",
			inputSchema: map[string]interface{}{
				"$schema":              "http://json-schema.org/draft-07/schema#",
				"type":                 "object",
				"additionalProperties": false,
				"description":          "An object with extra keywords.",
				"properties": map[string]interface{}{
					"value": map[string]interface{}{
						"type":             "number",
						"exclusiveMinimum": 0,
					},
				},
			},
			expectedSchema: &antigravity.GeminiParameterSchema{
				Type:        "OBJECT",
				Description: "An object with extra keywords.",
				Properties: map[string]*antigravity.GeminiParameterSchema{
					"value": {
						Type: "NUMBER",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualSchema := convertToGeminiSchema(tc.inputSchema)

			if !reflect.DeepEqual(actualSchema, tc.expectedSchema) {
				actualJSON, _ := json.MarshalIndent(actualSchema, "", "  ")
				expectedJSON, _ := json.MarshalIndent(tc.expectedSchema, "", "  ")
				t.Errorf("Schema conversion failed.\nExpected:\n%s\n\nGot:\n%s", string(expectedJSON), string(actualJSON))
			}
		})
	}
}
