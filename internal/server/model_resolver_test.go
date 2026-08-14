package server

import (
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
)

func TestResolveModelForThinking(t *testing.T) {
	testCases := []struct {
		name          string
		model         string
		thinkingLevel string
		expected      string
	}{
		{
			name:          "Gemini 3.1 Pro low",
			model:         "gemini-3.1-pro-preview",
			thinkingLevel: "LOW",
			expected:      "gemini-3.1-pro-low",
		},
		{
			name:          "Gemini 3.1 Pro medium falls back low",
			model:         "gemini-3.1-pro-preview",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.1-pro-low",
		},
		{
			name:          "Gemini 3.1 Pro fuzzy without prefix",
			model:         "3.1-pro-preview",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.1-pro-low",
		},
		{
			name:          "Gemini 3.1 Pro high uses agent model",
			model:         "gemini-3.1-pro-preview",
			thinkingLevel: "HIGH",
			expected:      "gemini-pro-agent",
		},
		{
			name:          "Exact Gemini 3.1 Pro high uses agent model",
			model:         "gemini-3.1-pro-high",
			thinkingLevel: "LOW",
			expected:      "gemini-pro-agent",
		},
		{
			name:          "Gemini 3 Flash ignores thinking",
			model:         "gemini-3-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3-flash",
		},
		{
			name:          "Gemini 3.5 Flash none falls back lowest",
			model:         "gemini-3.5-flash",
			thinkingLevel: "",
			expected:      "gemini-3.5-flash-extra-low",
		},
		{
			name:          "Gemini 3.5 Flash low",
			model:         "gemini-3.5-flash",
			thinkingLevel: "LOW",
			expected:      "gemini-3.5-flash-extra-low",
		},
		{
			name:          "Gemini 3.5 Flash minimal",
			model:         "3.5-flash",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-3.5-flash-extra-low",
		},
		{
			name:          "Gemini 3.5 Flash medium",
			model:         "gemini-3.5-flash",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.5-flash-low",
		},
		{
			name:          "Gemini 3.5 Flash high",
			model:         "gemini-3.5-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3-flash-agent",
		},
		{
			name:          "Exact Gemini 3.5 Flash high stays high",
			model:         "gemini-3-flash-agent",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-3-flash-agent",
		},
		{
			name:          "Exact Gemini 3.5 Flash medium stays medium",
			model:         "gemini-3.5-flash-low",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.5-flash-low",
		},
		{
			name:          "Gemini 3.1 Flash Lite canonical",
			model:         "gemini-3.1-flash-lite",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.1-flash-lite",
		},
		{
			name:          "Gemini 3.1 Flash Lite fuzzy without prefix",
			model:         "3.1-flash-lite-preview",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-3.1-flash-lite",
		},
		{
			name:          "Gemini 3.1 Flash Lite legacy alias direct match",
			model:         "gemini-2.5-flash-thinking",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-2.5-flash-thinking",
		},
		{
			name:          "Gemini 3.1 Flash Image",
			model:         "gemini-3.1-flash-image",
			thinkingLevel: "LOW",
			expected:      "gemini-3.1-flash-image",
		},
		{
			name:          "Gemini 3.7 Flash default",
			model:         "gemini-3.7-flash",
			thinkingLevel: "",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:          "Gemini 3.7 Flash low",
			model:         "gemini-3.7-flash",
			thinkingLevel: "LOW",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:          "Gemini 3.7 Flash medium",
			model:         "gemini-3.7-flash",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:          "Gemini 3.7 Flash high",
			model:         "gemini-3.7-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:          "Gemini 3.7 Flash tiered direct match",
			model:         "gemini-3.7-flash-tiered",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:          "Gemini 3.6 Flash default",
			model:         "gemini-3.6-flash",
			thinkingLevel: "",
			expected:      "gemini-3.6-flash-low",
		},
		{
			name:          "Gemini 3.6 Flash high",
			model:         "gemini-3.6-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.6-flash-high",
		},
		{
			name:          "Gemini 3.6 Flash tiered direct match",
			model:         "gemini-3.6-flash-tiered",
			thinkingLevel: "LOW",
			expected:      "gemini-3.6-flash-tiered",
		},
		{
			name:          "GPT-OSS 120B generic",
			model:         "gpt-oss-120b",
			thinkingLevel: "",
			expected:      "gpt-oss-120b-medium",
		},
		{
			name:          "GPT-OSS 120B medium direct match",
			model:         "gpt-oss-120b-medium",
			thinkingLevel: "HIGH",
			expected:      "gpt-oss-120b-medium",
		},
		{
			name:          "Unknown model passes through",
			model:         "custom-model",
			thinkingLevel: "HIGH",
			expected:      "custom-model",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveModelForThinking(tc.model, requestWithThinkingLevel(tc.thinkingLevel))
			if got != tc.expected {
				t.Fatalf("resolveModelForThinking(%q, %q) = %q, want %q", tc.model, tc.thinkingLevel, got, tc.expected)
			}
		})
	}
}

func requestWithThinkingLevel(level string) antigravity.GeminiInternalRequest {
	if level == "" {
		return antigravity.GeminiInternalRequest{}
	}
	return antigravity.GeminiInternalRequest{
		GenerationConfig: &antigravity.GeminiGenerationConfig{
			ThinkingConfig: &antigravity.ThinkingConfig{
				ThinkingLevel: level,
			},
		},
	}
}
