package server

import (
	"context"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
)

const (
	modelGemini3Flash               = "gemini-3-flash"
	modelGemini31FlashLite          = "gemini-3.1-flash-lite"
	modelGemini31FlashImage         = "gemini-3.1-flash-image"
	modelGemini35FlashLite          = "gemini-3.5-flash-lite"
	modelGemini38FlashLow           = "gemini-3.8-flash-low"
	modelGemini38FlashMedium        = "gemini-3.8-flash-medium"
	modelGemini38FlashHigh          = "gemini-3.8-flash-high"
	modelGemini38FlashTiered        = "gemini-3.8-flash-tiered"
	modelGemini4ArgonLow            = "gemini-4-argon-low"
	modelGemini4ArgonMedium         = "gemini-4-argon-medium"
	modelGemini4ArgonHigh           = "gemini-4-argon-high"
	modelGemini4CarbonLow           = "gemini-4-carbon-low"
	modelGemini4CarbonMedium        = "gemini-4-carbon-medium"
	modelGemini4CarbonHigh          = "gemini-4-carbon-high"
	modelGemini4HydrogenLow         = "gemini-4-hydrogen-low"
	modelGemini4HydrogenMedium      = "gemini-4-hydrogen-medium"
	modelGemini4HydrogenHigh        = "gemini-4-hydrogen-high"
	modelGemini4HydrogenXHigh       = "gemini-4-hydrogen-xhigh"
	modelGeminiFlashWorkerAgy       = "gemini-flash-worker-agy"
	modelGeminiProOrchestrator      = "gemini-pro-orchestrator"
	modelGeminiProOrchestratorArgon = "gemini-pro-orchestrator-argon"
)

func resolveModelForThinking(model string, req antigravity.GeminiInternalRequest) string {
	modelLower := strings.ToLower(strings.TrimSpace(model))
	if isKnownUpstreamModelID(modelLower) {
		return modelLower
	}

	thinkingLevel := normalizedThinkingLevel(req)
	switch thinkingLevel {
	case "none", "off":
		thinkingLevel = "none"
	case "unspecified", "thinking_level_unspecified":
		thinkingLevel = "unspecified"
	}

	switch {
	case isGeminiOrchestratorModel(modelLower):
		if strings.Contains(modelLower, "argon") {
			return modelGeminiProOrchestratorArgon
		}
		return modelGeminiProOrchestrator

	case isGemini4ArgonModel(modelLower):
		return resolveGemini4EffortModel(thinkingLevel, req, modelGemini4ArgonLow, modelGemini4ArgonMedium, modelGemini4ArgonHigh)

	case isGemini4CarbonModel(modelLower):
		return resolveGemini4EffortModel(thinkingLevel, req, modelGemini4CarbonLow, modelGemini4CarbonMedium, modelGemini4CarbonHigh)

	case isGemini4HydrogenModel(modelLower):
		if !thinkingBudgetDisabled(req) && (strings.HasSuffix(modelLower, "-extra") || strings.HasSuffix(modelLower, "-extra-high") || thinkingLevel == "extra" || thinkingLevel == "xhigh") {
			return modelGemini4HydrogenXHigh
		}
		return resolveGemini4EffortModel(thinkingLevel, req, modelGemini4HydrogenLow, modelGemini4HydrogenMedium, modelGemini4HydrogenHigh)

	case isGemini38FlashModel(modelLower):
		if thinkingBudgetDisabled(req) {
			return modelGemini38FlashTiered
		}
		switch thinkingLevel {
		case "none", "unspecified":
			return modelGemini38FlashTiered
		case "high", "extra", "xhigh":
			return modelGemini38FlashHigh
		case "medium":
			return modelGemini38FlashMedium
		case "minimal", "low", "":
			return modelGemini38FlashLow
		default:
			return modelGemini38FlashLow
		}

	case isGeminiFlashLiteModel(modelLower):
		if strings.Contains(modelLower, "3.5-flash-lite") {
			return modelGemini35FlashLite
		}
		return modelGemini31FlashLite

	default:
		return model
	}
}

func isKnownUpstreamModelID(modelLower string) bool {
	switch modelLower {
	case "gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash-thinking",
		"gemini-2.5-pro",
		modelGemini3Flash,
		modelGemini31FlashImage,
		modelGemini31FlashLite,
		modelGemini35FlashLite,
		modelGemini38FlashLow,
		modelGemini38FlashMedium,
		modelGemini38FlashHigh,
		modelGemini38FlashTiered,
		modelGemini4ArgonLow,
		modelGemini4ArgonMedium,
		modelGemini4ArgonHigh,
		modelGemini4CarbonLow,
		modelGemini4CarbonMedium,
		modelGemini4CarbonHigh,
		modelGemini4HydrogenLow,
		modelGemini4HydrogenMedium,
		modelGemini4HydrogenHigh,
		modelGemini4HydrogenXHigh,
		modelGeminiFlashWorkerAgy,
		modelGeminiProOrchestrator,
		modelGeminiProOrchestratorArgon:
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

func thinkingBudgetDisabled(req antigravity.GeminiInternalRequest) bool {
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		return false
	}
	budget := req.GenerationConfig.ThinkingConfig.ThinkingBudget
	return budget != nil && *budget == 0
}

// resolveGemini4EffortModel maps a Gemini 4 model family to an effort-suffixed
// ID. Upstream only serves the effort-suffixed IDs; the bare name 404s. Without
// an explicit level, use medium, which upstream reports as its default agent
// model. Thinking-off requests (a disabling level or thinkingBudget 0) use low,
// and the budget-0 config is passed through.
func resolveGemini4EffortModel(thinkingLevel string, req antigravity.GeminiInternalRequest, low, medium, high string) string {
	if thinkingBudgetDisabled(req) {
		return low
	}
	switch thinkingLevel {
	case "high", "extra", "xhigh":
		return high
	case "minimal", "low", "none", "unspecified":
		return low
	default:
		return medium
	}
}

func resolveModelWithCatalog(model string, req antigravity.GeminiInternalRequest, catalog map[string]antigravity.AvailableModel) string {
	resolved := resolveModelForThinking(model, req)
	if resolved != model || len(catalog) == 0 {
		return resolved
	}

	modelLower := strings.ToLower(strings.TrimSpace(model))
	if isKnownUpstreamModelID(modelLower) || modelFamily(modelLower) != "gemini" || !isSupportedModel(modelLower) {
		return resolved
	}
	if _, exists := catalog[modelLower]; exists {
		return modelLower
	}

	thinkingLevel := normalizedThinkingLevel(req)
	var suffixes []string
	switch {
	case thinkingBudgetDisabled(req) || thinkingLevel == "none" || thinkingLevel == "off" || thinkingLevel == "unspecified" || thinkingLevel == "thinking_level_unspecified":
		suffixes = []string{"-tiered", "-low", "-medium", "-high", "-xhigh"}
	case thinkingLevel == "extra" || thinkingLevel == "xhigh":
		suffixes = []string{"-xhigh", "-high", "-medium", "-low"}
	case thinkingLevel == "high":
		suffixes = []string{"-high", "-medium", "-low", "-xhigh"}
	case thinkingLevel == "minimal" || thinkingLevel == "low":
		suffixes = []string{"-low", "-medium", "-high", "-xhigh"}
	default:
		suffixes = []string{"-medium", "-high", "-low", "-xhigh"}
	}

	for _, suffix := range suffixes {
		candidate := modelLower + suffix
		if _, ok := catalog[candidate]; ok && isSupportedModel(candidate) {
			return candidate
		}
	}
	return resolved
}

func (s *Server) resolveRequestModel(ctx context.Context, model string, req antigravity.GeminiInternalRequest) string {
	resolved := resolveModelForThinking(model, req)
	if resolved != model {
		return resolved
	}

	modelLower := strings.ToLower(strings.TrimSpace(model))
	if isKnownUpstreamModelID(modelLower) || modelFamily(modelLower) != "gemini" || !isSupportedModel(modelLower) || s == nil || s.antigravityClient == nil {
		return resolved
	}

	data, err := s.antigravityClient.FetchAvailableModels(ctx)
	if err != nil || data == nil {
		return resolved
	}
	return resolveModelWithCatalog(model, req, data.Models)
}

func isGeminiOrchestratorModel(modelLower string) bool {
	return strings.Contains(modelLower, "orchestrator") &&
		(strings.Contains(modelLower, "gemini") || strings.HasPrefix(modelLower, "orchestrator"))
}

func isGemini4ArgonModel(modelLower string) bool {
	return strings.Contains(modelLower, "gemini") && strings.Contains(modelLower, "argon")
}

func isGemini4CarbonModel(modelLower string) bool {
	return strings.Contains(modelLower, "gemini") && strings.Contains(modelLower, "carbon")
}

func isGemini4HydrogenModel(modelLower string) bool {
	return strings.Contains(modelLower, "gemini") && strings.Contains(modelLower, "hydrogen")
}

func isGemini38FlashModel(modelLower string) bool {
	return strings.Contains(modelLower, "3.8-flash")
}

func isGeminiFlashLiteModel(modelLower string) bool {
	return strings.Contains(modelLower, "flash-lite")
}

func applyModelThinkingDefaults(requestedModel string, req *antigravity.GeminiInternalRequest) {
	if req == nil {
		return
	}
	modelLower := strings.ToLower(strings.TrimSpace(requestedModel))
	if !strings.Contains(modelLower, "3.8-flash") {
		return
	}
	if req.GenerationConfig == nil {
		req.GenerationConfig = &antigravity.GeminiGenerationConfig{}
	}
	if req.GenerationConfig.ThinkingConfig == nil {
		req.GenerationConfig.ThinkingConfig = &antigravity.ThinkingConfig{}
	}

	// If thinkingBudget is explicitly 0, or thinkingLevel is disabled/unspecified, do not override with defaults
	if req.GenerationConfig.ThinkingConfig.ThinkingBudget != nil && *req.GenerationConfig.ThinkingConfig.ThinkingBudget == 0 {
		return
	}
	currentLevel := strings.ToLower(strings.TrimSpace(req.GenerationConfig.ThinkingConfig.ThinkingLevel))
	if currentLevel == "none" || currentLevel == "off" || currentLevel == "thinking_level_unspecified" || currentLevel == "unspecified" {
		return
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
