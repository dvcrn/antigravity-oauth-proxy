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

	level := ""
	switch {
	case strings.Contains(modelLower, "-low"):
		level = "low"
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
