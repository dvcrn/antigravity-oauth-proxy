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
	modelGemini36FlashLow     = "gemini-3.6-flash-low"
	modelGemini36FlashMedium  = "gemini-3.6-flash-medium"
	modelGemini36FlashHigh    = "gemini-3.6-flash-high"
	modelGemini36FlashTiered  = "gemini-3.6-flash-tiered"
	modelGemini37FlashLow     = "gemini-3.7-flash-low"
	modelGemini37FlashMedium  = "gemini-3.7-flash-medium"
	modelGemini37FlashHigh    = "gemini-3.7-flash-high"
	modelGemini37FlashTiered  = "gemini-3.7-flash-tiered"
	modelGemini31FlashLite    = "gemini-3.1-flash-lite"
	modelGemini31FlashImage   = "gemini-3.1-flash-image"
	modelGptOss120bMedium     = "gpt-oss-120b-medium"
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

	case isGemini37FlashModel(modelLower):
		return modelGemini37FlashTiered

	case isGemini36FlashModel(modelLower):
		switch thinkingLevel {
		case "high":
			return modelGemini36FlashHigh
		case "medium":
			return modelGemini36FlashMedium
		case "minimal", "low", "":
			return modelGemini36FlashLow
		default:
			return modelGemini36FlashLow
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

	case isGptOssModel(modelLower):
		return modelGptOss120bMedium

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
		modelGemini36FlashHigh,
		modelGemini36FlashMedium,
		modelGemini36FlashLow,
		modelGemini36FlashTiered,
		modelGemini37FlashTiered,
		modelGemini31ProHighAgent,
		modelGptOss120bMedium:
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

func isGemini37FlashModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.7-flash") ||
		modelLower == modelGemini37FlashHigh ||
		modelLower == modelGemini37FlashMedium ||
		modelLower == modelGemini37FlashLow ||
		modelLower == modelGemini37FlashTiered
}

func isGemini36FlashModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.6-flash") ||
		modelLower == modelGemini36FlashHigh ||
		modelLower == modelGemini36FlashMedium ||
		modelLower == modelGemini36FlashLow ||
		modelLower == modelGemini36FlashTiered
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

func isGptOssModel(modelLower string) bool {
	return strings.Contains(modelLower, "gpt-oss")
}

func applyModelThinkingDefaults(requestedModel string, req *antigravity.GeminiInternalRequest) {
	if req == nil {
		return
	}
	modelLower := strings.ToLower(strings.TrimSpace(requestedModel))
	if !strings.Contains(modelLower, "3.7-flash") {
		return
	}
	if req.GenerationConfig == nil {
		req.GenerationConfig = &antigravity.GeminiGenerationConfig{}
	}
	if req.GenerationConfig.ThinkingConfig == nil {
		req.GenerationConfig.ThinkingConfig = &antigravity.ThinkingConfig{}
	}
	if req.GenerationConfig.ThinkingConfig.ThinkingLevel == "" {
		switch {
		case strings.Contains(modelLower, "-high"):
			req.GenerationConfig.ThinkingConfig.ThinkingLevel = "HIGH"
		case strings.Contains(modelLower, "-medium"):
			req.GenerationConfig.ThinkingConfig.ThinkingLevel = "MEDIUM"
		default:
			req.GenerationConfig.ThinkingConfig.ThinkingLevel = "LOW"
		}
	}
}
