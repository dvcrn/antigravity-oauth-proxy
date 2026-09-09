package antigravity

import (
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

func applyGeminiThinkingPreset(req *GenerateContentRequest) {
	if req == nil {
		return
	}

	modelLower := strings.ToLower(req.Model)
	if !strings.Contains(modelLower, "gemini") {
		return
	}

	// If thinkingBudget is explicitly 0, or thinkingLevel is disabled/unspecified, do not override with preset
	if req.Request.GenerationConfig != nil && req.Request.GenerationConfig.ThinkingConfig != nil {
		tc := req.Request.GenerationConfig.ThinkingConfig
		if tc.ThinkingBudget != nil && *tc.ThinkingBudget == 0 {
			return
		}
		curLevel := strings.ToLower(strings.TrimSpace(tc.ThinkingLevel))
		if curLevel == "none" || curLevel == "off" || curLevel == "thinking_level_unspecified" || curLevel == "unspecified" {
			return
		}
	}

	level := ""
	switch {
	case strings.Contains(modelLower, "-low"):
		level = "low"
	case strings.Contains(modelLower, "-medium"):
		level = "medium"
	case strings.Contains(modelLower, "-high"):
		level = "high"
	}
	if level == "" {
		return
	}

	logger.Get().Info().
		Str("model", req.Model).
		Str("thinking_level", level).
		Msg("Applied Gemini thinking preset")

	if req.Request.GenerationConfig == nil {
		req.Request.GenerationConfig = &GeminiGenerationConfig{}
	}
	if req.Request.GenerationConfig.ThinkingConfig == nil {
		req.Request.GenerationConfig.ThinkingConfig = &ThinkingConfig{}
	}

	req.Request.GenerationConfig.ThinkingConfig.ThinkingLevel = level
}
