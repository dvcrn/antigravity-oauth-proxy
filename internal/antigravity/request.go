package antigravity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
	"github.com/google/uuid"
)

func prepareAntigravityRequest(req *GenerateContentRequest) {
	if req == nil {
		return
	}

	req.UserAgent = RequestUserAgent
	req.RequestType = RequestTypeAgent
	if req.RequestID == "" {
		req.RequestID = newAntigravityRequestID()
	}

	if req.Request.SessionID == "" {
		req.Request.SessionID = deriveSessionID(req.Request.Contents)
	}

	if prunedParts, prunedContents := sanitizeContents(&req.Request.Contents); prunedParts > 0 || prunedContents > 0 {
		logger.Get().Warn().
			Int("pruned_parts", prunedParts).
			Int("pruned_contents", prunedContents).
			Msg("Removed empty content parts from request")
	}

	if modelEncodesThinkingLevel(req.Model) {
		clearRequestThinkingLevel(req)
	} else {
		applyGeminiThinkingPreset(req)
	}
	ensureAntigravityThinkingDefaults(req)

	if missing := fillMissingParameters(req.Request.Tools); missing > 0 {
		logger.Get().Warn().
			Int("missing_parameters", missing).
			Str("missing_names", missingParameterNames(req.Request.Tools, 6)).
			Msg("Defaulted missing parameters in request tools")
	}

	if missing := ensureFunctionCallIDs(req.Request.Contents); missing > 0 {
		logger.Get().Warn().
			Int("missing_ids", missing).
			Msg("Defaulted missing functionCall IDs in request contents")
	}

	if missing := ensureFunctionResponseIDs(req.Request.Contents); missing > 0 {
		logger.Get().Warn().
			Int("missing_ids", missing).
			Msg("Defaulted missing functionResponse IDs in request contents")
	}

	req.Request.SystemInstruction = buildAntigravitySystemInstruction(req.Request.SystemInstruction)
	logPreparedThinkingConfig(req)
}

func logPreparedThinkingConfig(req *GenerateContentRequest) {
	if req == nil {
		return
	}

	maxOutputTokens := 0
	temperature := 0.0
	thinkingLevel := ""
	includeThoughts := false
	var thinkingBudget interface{}
	hasGenerationConfig := req.Request.GenerationConfig != nil
	hasThinkingConfig := false
	if req.Request.GenerationConfig != nil {
		maxOutputTokens = req.Request.GenerationConfig.MaxOutputTokens
		temperature = req.Request.GenerationConfig.Temperature
		if req.Request.GenerationConfig.ThinkingConfig != nil {
			hasThinkingConfig = true
			thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
			thinkingLevel = thinkingConfig.ThinkingLevel
			if thinkingConfig.IncludeThoughts != nil {
				includeThoughts = *thinkingConfig.IncludeThoughts
			}
			if thinkingConfig.ThinkingBudget != nil {
				thinkingBudget = *thinkingConfig.ThinkingBudget
			}
		}
	}

	logger.Get().Info().
		Str("model", req.Model).
		Bool("has_generation_config", hasGenerationConfig).
		Bool("has_thinking_config", hasThinkingConfig).
		Str("thinking_level", thinkingLevel).
		Interface("thinking_budget", thinkingBudget).
		Bool("include_thoughts", includeThoughts).
		Int("max_output_tokens", maxOutputTokens).
		Float64("temperature", temperature).
		Msg("Prepared CloudCode thinking config")
}

func modelEncodesThinkingLevel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gemini-3.1-pro-low",
		"gemini-3.1-pro-high",
		"gemini-3.5-flash-extra-low",
		"gemini-3.5-flash-low",
		"gemini-3-flash-agent",
		"gemini-pro-agent":
		return true
	default:
		return false
	}
}

func clearRequestThinkingLevel(req *GenerateContentRequest) {
	if req == nil || req.Request.GenerationConfig == nil || req.Request.GenerationConfig.ThinkingConfig == nil {
		return
	}

	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if strings.TrimSpace(thinkingConfig.ThinkingLevel) == "" {
		return
	}

	logger.Get().Info().
		Str("model", req.Model).
		Str("thinking_level", thinkingConfig.ThinkingLevel).
		Msg("Cleared explicit thinking level because model ID encodes thinking effort")
	thinkingConfig.ThinkingLevel = ""
}

func ensureAntigravityThinkingDefaults(req *GenerateContentRequest) {
	if req == nil {
		return
	}
	if req.Request.GenerationConfig == nil {
		req.Request.GenerationConfig = &GeminiGenerationConfig{}
	}
	if req.Request.GenerationConfig.ThinkingConfig == nil {
		req.Request.GenerationConfig.ThinkingConfig = &ThinkingConfig{}
	}

	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if thinkingConfig.IncludeThoughts == nil {
		includeThoughts := true
		thinkingConfig.IncludeThoughts = &includeThoughts
	}
	if thinkingConfig.ThinkingBudget == nil {
		thinkingBudget := 10001
		thinkingConfig.ThinkingBudget = &thinkingBudget
	}
}

func buildAntigravitySystemInstruction(existing *SystemInstruction) *SystemInstruction {
	parts := []ContentPart{
		{Text: strings.TrimSpace(SystemInstructionText)},
	}

	if existing != nil {
		for _, part := range existing.Parts {
			if part.Text != "" {
				parts = append(parts, ContentPart{Text: part.Text})
			}
		}
	}

	return &SystemInstruction{
		Role:  "user",
		Parts: parts,
	}
}

func newAntigravityRequestID() string {
	conversationID := uuid.NewString()
	trajectoryID := uuid.NewString()
	return fmt.Sprintf("agent/%s/%d/%s/1", conversationID, time.Now().UnixMilli(), trajectoryID)
}

func deriveSessionID(contents []Content) string {
	for _, msg := range contents {
		if strings.ToLower(msg.Role) != "user" {
			continue
		}

		var parts []string
		for _, part := range msg.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}

		if len(parts) == 0 {
			continue
		}

		sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
		return hex.EncodeToString(sum[:])[:32]
	}

	return uuid.NewString()
}

func ensureFunctionCallIDs(contents []Content) int {
	missing := 0
	for contentIndex := range contents {
		for partIndex := range contents[contentIndex].Parts {
			part := contents[contentIndex].Parts[partIndex]
			if part.FunctionCall == nil {
				continue
			}
			if strings.TrimSpace(part.FunctionCall.ID) == "" {
				contents[contentIndex].Parts[partIndex].FunctionCall.ID = "toolu_" + uuid.NewString()
				missing++
			}
		}
	}
	return missing
}

func ensureFunctionResponseIDs(contents []Content) int {
	missing := 0
	var pending []string
	perName := map[string][]string{}

	for contentIndex := range contents {
		for partIndex := range contents[contentIndex].Parts {
			part := &contents[contentIndex].Parts[partIndex]
			if part.FunctionCall != nil {
				id := strings.TrimSpace(part.FunctionCall.ID)
				if id == "" {
					continue
				}
				pending = append(pending, id)
				if name := strings.TrimSpace(part.FunctionCall.Name); name != "" {
					perName[name] = append(perName[name], id)
				}
				continue
			}

			if part.FunctionResponse == nil {
				continue
			}

			if strings.TrimSpace(part.FunctionResponse.ID) != "" {
				// Consume any queued IDs so we don't reuse them later.
				pending = removeFirstMatch(pending, part.FunctionResponse.ID)
				if name := strings.TrimSpace(part.FunctionResponse.Name); name != "" {
					perName[name] = removeFirstMatch(perName[name], part.FunctionResponse.ID)
				}
				continue
			}

			assigned := ""
			if name := strings.TrimSpace(part.FunctionResponse.Name); name != "" {
				if ids := perName[name]; len(ids) > 0 {
					assigned = ids[0]
					perName[name] = ids[1:]
					pending = removeFirstMatch(pending, assigned)
				}
			}
			if assigned == "" && len(pending) > 0 {
				assigned = pending[0]
				pending = pending[1:]
				if name := strings.TrimSpace(part.FunctionResponse.Name); name != "" {
					perName[name] = removeFirstMatch(perName[name], assigned)
				}
			}

			if assigned != "" {
				part.FunctionResponse.ID = assigned
				missing++
			}
		}
	}

	return missing
}

func removeFirstMatch(values []string, target string) []string {
	if target == "" {
		return values
	}
	for i, v := range values {
		if v == target {
			return append(values[:i], values[i+1:]...)
		}
	}
	return values
}

func sanitizeContents(contents *[]Content) (int, int) {
	if contents == nil || len(*contents) == 0 {
		return 0, 0
	}

	prunedParts := 0
	prunedContents := 0
	cleaned := make([]Content, 0, len(*contents))
	for _, content := range *contents {
		if len(content.Parts) == 0 {
			prunedContents++
			continue
		}

		parts := make([]ContentPart, 0, len(content.Parts))
		for _, part := range content.Parts {
			if isEmptyContentPart(part) {
				prunedParts++
				continue
			}
			parts = append(parts, part)
		}

		if len(parts) == 0 {
			prunedContents++
			continue
		}
		content.Parts = parts
		cleaned = append(cleaned, content)
	}

	*contents = cleaned
	return prunedParts, prunedContents
}

func isEmptyContentPart(part ContentPart) bool {
	if part.FunctionCall != nil || part.FunctionResponse != nil {
		return false
	}
	return part.Text == ""
}
