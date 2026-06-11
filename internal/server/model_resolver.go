package server

import (
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
)

const (
	modelGemini31ProLow       = "gemini-3.1-pro-low"
	modelGemini31ProHigh      = "gemini-3.1-pro-high"
	modelGemini31ProHighAgent = "gemini-pro-agent"
	modelGemini3Flash         = "gemini-3-flash"
	modelGemini35FlashLow     = "gemini-3.5-flash-extra-low"
	modelGemini35FlashMedium  = "gemini-3.5-flash-low"
	modelGemini35FlashHigh    = "gemini-3-flash-agent"
	modelGemini31FlashLite    = "gemini-3.1-flash-lite"
	modelGemini31FlashImage   = "gemini-3.1-flash-image"
)

func resolveModelForThinking(model string, req antigravity.GeminiInternalRequest) string {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if modelLower == modelGemini31ProHigh {
		return modelGemini31ProHighAgent
	}
	if isKnownUpstreamModelID(modelLower) {
		return modelLower
	}

	thinkingLevel := normalizedThinkingLevel(req)

	switch {
	case isGemini31ProModel(modelLower):
		switch thinkingLevel {
		case "high":
			return modelGemini31ProHighAgent
		case "minimal", "low", "medium", "":
			return modelGemini31ProLow
		default:
			return modelGemini31ProLow
		}

	case isGemini35FlashModel(modelLower):
		switch thinkingLevel {
		case "high":
			return modelGemini35FlashHigh
		case "medium":
			return modelGemini35FlashMedium
		case "minimal", "low", "":
			return modelGemini35FlashLow
		default:
			return modelGemini35FlashLow
		}

	case isGemini31FlashLiteModel(modelLower):
		return modelGemini31FlashLite

	case modelLower == modelGemini31FlashImage:
		return modelGemini31FlashImage

	case modelLower == modelGemini3Flash:
		return modelGemini3Flash

	default:
		return model
	}
}

func isKnownUpstreamModelID(modelLower string) bool {
	switch modelLower {
	case "claude-opus-4-6-thinking",
		"claude-sonnet-4-6",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash-thinking",
		"gemini-2.5-pro",
		modelGemini3Flash,
		modelGemini35FlashHigh,
		modelGemini31FlashImage,
		modelGemini31FlashLite,
		modelGemini31ProHigh,
		modelGemini31ProLow,
		modelGemini35FlashLow,
		modelGemini35FlashMedium,
		modelGemini31ProHighAgent:
		return true
	default:
		return false
	}
}

func normalizedThinkingLevel(req antigravity.GeminiInternalRequest) string {
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(req.GenerationConfig.ThinkingConfig.ThinkingLevel))
}

func isGemini31ProModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.1-pro") || modelLower == "gemini-pro-agent"
}

func isGemini35FlashModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.5-flash") || modelLower == modelGemini35FlashHigh
}

func isGemini31FlashLiteModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.1-flash-lite") ||
		modelLower == "gemini-2.5-flash" ||
		modelLower == "gemini-2.5-flash-lite" ||
		modelLower == "gemini-2.5-flash-thinking"
}
