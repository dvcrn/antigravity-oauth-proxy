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
			name:          "Gemini 3.5 Flash Lite canonical",
			model:         "gemini-3.5-flash-lite",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.5-flash-lite",
		},
		{
			name:          "Gemini 3.5 Flash Lite off",
			model:         "gemini-3.5-flash-lite",
			thinkingLevel: "off",
			expected:      "gemini-3.5-flash-lite",
		},
		{
			name:          "Gemini 3.7 Flash default",
			model:         "gemini-3.7-flash",
			thinkingLevel: "",
			expected:      "gemini-3.7-flash-low",
		},
		{
			name:          "Gemini 3.7 Flash low",
			model:         "gemini-3.7-flash",
			thinkingLevel: "LOW",
			expected:      "gemini-3.7-flash-low",
		},
		{
			name:          "Gemini 3.7 Flash medium",
			model:         "gemini-3.7-flash",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.7-flash-medium",
		},
		{
			name:          "Gemini 3.7 Flash high",
			model:         "gemini-3.7-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.7-flash-high",
		},
		{
			name:          "Gemini 3.7 Flash tiered direct match",
			model:         "gemini-3.7-flash-tiered",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.7-flash-tiered",
		},
		{
			name:     "Gemini 4 Argon bare defaults to medium",
			model:    "gemini-4-argon",
			expected: "gemini-4-argon-medium",
		},
		{
			name:          "Gemini 4 Argon low",
			model:         "gemini-4-argon",
			thinkingLevel: "LOW",
			expected:      "gemini-4-argon-low",
		},
		{
			name:          "Gemini 4 Argon minimal maps to low",
			model:         "gemini-4-argon",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-4-argon-low",
		},
		{
			name:          "Gemini 4 Argon thinking off maps to low",
			model:         "gemini-4-argon",
			thinkingLevel: "NONE",
			expected:      "gemini-4-argon-low",
		},
		{
			name:          "Gemini 4 Argon medium",
			model:         "gemini-4-argon-preview",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-4-argon-medium",
		},
		{
			name:          "Gemini 4 Argon high",
			model:         "gemini-4-argon",
			thinkingLevel: "HIGH",
			expected:      "gemini-4-argon-high",
		},
		{
			name:          "Gemini 4 Argon direct ID wins over level",
			model:         "gemini-4-argon-high",
			thinkingLevel: "LOW",
			expected:      "gemini-4-argon-high",
		},
		{
			name:     "Gemini 4 Carbon bare defaults to medium",
			model:    "gemini-4-carbon",
			expected: "gemini-4-carbon-medium",
		},
		{
			name:          "Gemini 4 Carbon low",
			model:         "gemini-4-carbon",
			thinkingLevel: "LOW",
			expected:      "gemini-4-carbon-low",
		},
		{
			name:          "Gemini 4 Carbon minimal maps to low",
			model:         "gemini-4-carbon",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-4-carbon-low",
		},
		{
			name:          "Gemini 4 Carbon thinking off maps to low",
			model:         "gemini-4-carbon",
			thinkingLevel: "NONE",
			expected:      "gemini-4-carbon-low",
		},
		{
			name:          "Gemini 4 Carbon medium",
			model:         "gemini-4-carbon-preview",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-4-carbon-medium",
		},
		{
			name:          "Gemini 4 Carbon high",
			model:         "gemini-4-carbon",
			thinkingLevel: "HIGH",
			expected:      "gemini-4-carbon-high",
		},
		{
			name:          "Gemini 4 Carbon direct ID wins over level",
			model:         "gemini-4-carbon-high",
			thinkingLevel: "LOW",
			expected:      "gemini-4-carbon-high",
		},
		{
			name:     "Gemini 4 Hydrogen bare defaults to medium",
			model:    "gemini-4-hydrogen",
			expected: "gemini-4-hydrogen-medium",
		},
		{
			name:          "Gemini 4 Hydrogen low",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "LOW",
			expected:      "gemini-4-hydrogen-low",
		},
		{
			name:          "Gemini 4 Hydrogen minimal maps to low",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "MINIMAL",
			expected:      "gemini-4-hydrogen-low",
		},
		{
			name:          "Gemini 4 Hydrogen thinking off maps to low",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "NONE",
			expected:      "gemini-4-hydrogen-low",
		},
		{
			name:          "Gemini 4 Hydrogen medium",
			model:         "gemini-4-hydrogen-preview",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-4-hydrogen-medium",
		},
		{
			name:          "Gemini 4 Hydrogen high",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "HIGH",
			expected:      "gemini-4-hydrogen-high",
		},
		{
			name:          "Gemini 4 Hydrogen extra maps to xhigh",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "EXTRA",
			expected:      "gemini-4-hydrogen-xhigh",
		},
		{
			name:          "Gemini 4 Hydrogen xhigh maps to xhigh",
			model:         "gemini-4-hydrogen",
			thinkingLevel: "XHIGH",
			expected:      "gemini-4-hydrogen-xhigh",
		},
		{
			name:          "Gemini 4 Hydrogen -extra suffix maps to xhigh",
			model:         "gemini-4-hydrogen-extra",
			thinkingLevel: "LOW",
			expected:      "gemini-4-hydrogen-xhigh",
		},
		{
			name:          "Gemini 4 Hydrogen direct ID wins over level",
			model:         "gemini-4-hydrogen-xhigh",
			thinkingLevel: "LOW",
			expected:      "gemini-4-hydrogen-xhigh",
		},
		{
			name:          "Gemini Pro Orchestrator passes through",
			model:         "gemini-pro-orchestrator",
			thinkingLevel: "HIGH",
			expected:      "gemini-pro-orchestrator",
		},
		{
			name:          "Gemini Pro Orchestrator Argon passes through",
			model:         "gemini-pro-orchestrator-argon",
			thinkingLevel: "HIGH",
			expected:      "gemini-pro-orchestrator-argon",
		},
		{
			name:          "Gemini Pro Orchestrator Carbon alias resolves to default orchestrator",
			model:         "gemini-pro-orchestrator-carbon",
			thinkingLevel: "HIGH",
			expected:      "gemini-pro-orchestrator",
		},
		{
			name:          "Orchestrator alias resolves to default orchestrator",
			model:         "orchestrator",
			thinkingLevel: "LOW",
			expected:      "gemini-pro-orchestrator",
		},
		{
			name:          "Orchestrator Carbon alias resolves to default orchestrator",
			model:         "orchestrator-carbon",
			thinkingLevel: "LOW",
			expected:      "gemini-pro-orchestrator",
		},
		{
			name:          "Orchestrator Argon alias resolves to argon orchestrator",
			model:         "orchestrator-argon",
			thinkingLevel: "LOW",
			expected:      "gemini-pro-orchestrator-argon",
		},
		{
			name:          "Gemini 3.8 Flash default",
			model:         "gemini-3.8-flash",
			thinkingLevel: "",
			expected:      "gemini-3.8-flash-low",
		},
		{
			name:          "Gemini 3.8 Flash low",
			model:         "gemini-3.8-flash",
			thinkingLevel: "LOW",
			expected:      "gemini-3.8-flash-low",
		},
		{
			name:          "Gemini 3.8 Flash medium",
			model:         "gemini-3.8-flash",
			thinkingLevel: "MEDIUM",
			expected:      "gemini-3.8-flash-medium",
		},
		{
			name:          "Gemini 3.8 Flash high",
			model:         "gemini-3.8-flash",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.8-flash-high",
		},
		{
			name:          "Gemini 3.8 Flash tiered direct match",
			model:         "gemini-3.8-flash-tiered",
			thinkingLevel: "HIGH",
			expected:      "gemini-3.8-flash-tiered",
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
			name:          "Claude Sonnet 5.5 passes through",
			model:         "claude-sonnet-5-5",
			thinkingLevel: "HIGH",
			expected:      "claude-sonnet-5-5",
		},
		{
			name:          "Claude Opus 5.5 passes through",
			model:         "claude-opus-5-5",
			thinkingLevel: "HIGH",
			expected:      "claude-opus-5-5",
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

func TestResolveModelForThinkingArgonBudgetZeroUsesLow(t *testing.T) {
	zero := 0
	req := antigravity.GeminiInternalRequest{
		GenerationConfig: &antigravity.GeminiGenerationConfig{
			ThinkingConfig: &antigravity.ThinkingConfig{ThinkingBudget: &zero},
		},
	}
	if got := resolveModelForThinking("gemini-4-argon", req); got != modelGemini4ArgonLow {
		t.Fatalf("resolveModelForThinking(gemini-4-argon, budget 0) = %q, want %q", got, modelGemini4ArgonLow)
	}
	if got := resolveModelForThinking("gemini-4-carbon", req); got != modelGemini4CarbonLow {
		t.Fatalf("resolveModelForThinking(gemini-4-carbon, budget 0) = %q, want %q", got, modelGemini4CarbonLow)
	}
	if got := resolveModelForThinking("gemini-4-hydrogen", req); got != modelGemini4HydrogenLow {
		t.Fatalf("resolveModelForThinking(gemini-4-hydrogen, budget 0) = %q, want %q", got, modelGemini4HydrogenLow)
	}
}

func TestResolveModelForThinkingIgnoresNonGeminiArgon(t *testing.T) {
	if got := resolveModelForThinking("acme-argon-7b", antigravity.GeminiInternalRequest{}); got != "acme-argon-7b" {
		t.Fatalf("resolveModelForThinking(acme-argon-7b) = %q, want unchanged", got)
	}
	if got := resolveModelForThinking("acme-carbon-7b", antigravity.GeminiInternalRequest{}); got != "acme-carbon-7b" {
		t.Fatalf("resolveModelForThinking(acme-carbon-7b) = %q, want unchanged", got)
	}
	if got := resolveModelForThinking("acme-hydrogen-7b", antigravity.GeminiInternalRequest{}); got != "acme-hydrogen-7b" {
		t.Fatalf("resolveModelForThinking(acme-hydrogen-7b) = %q, want unchanged", got)
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
