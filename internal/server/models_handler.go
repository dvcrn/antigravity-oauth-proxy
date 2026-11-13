package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

type openAIModel struct {
	ID                 string `json:"id"`
	Object             string `json:"object"`
	Created            int64  `json:"created"`
	OwnedBy            string `json:"owned_by"`
	Name               string `json:"name"`
	Vendor             string `json:"vendor"`
	Version            string `json:"version"`
	ModelPickerEnabled bool   `json:"model_picker_enabled"`
	Preview            bool   `json:"preview"`
	Capabilities       any    `json:"capabilities"`
}

type openAIModelsListResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type geminiModel struct {
	Name                       string   `json:"name"`
	BaseModelID                string   `json:"baseModelId"`
	Version                    string   `json:"version"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit,omitempty"`
	OutputTokenLimit           int      `json:"outputTokenLimit,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Thinking                   bool     `json:"thinking"`
}

type geminiModelsListResponse struct {
	Models []geminiModel `json:"models"`
}

type apiErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelIDs, catalog, err := s.supportedModels(r.Context())
	if err != nil {
		logger.Get().Error().Err(err).Msg("Failed to fetch available models")
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// One timestamp for the whole listing: a per-model time.Now() lets entries in
	// the same response straddle a second boundary and report different values.
	created := time.Now().Unix()

	models := make([]openAIModel, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, newOpenAIModel(modelID, catalog[modelID], created))
	}

	// Handle request for a single model, e.g., /v1/models/gemini-2.5-pro
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) > 2 {
		requestedModelID := pathParts[2]
		for _, m := range models {
			if m.ID == requestedModelID {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(m)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	resp := openAIModelsListResponse{
		Object: "list",
		Data:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// geminiModelsHandler serves the Gemini API models.list and models.get shapes
// at /v1beta/models and /v1beta/models/{model}.
func (s *Server) geminiModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelIDs, catalog, err := s.supportedModels(r.Context())
	if err != nil {
		logger.Get().Error().Err(err).Msg("Failed to fetch available models")
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	models := make([]geminiModel, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, newGeminiModel(modelID, catalog[modelID]))
	}

	w.Header().Set("Content-Type", "application/json")
	requestedModelID := strings.Trim(strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/v1beta"), "/models"), "/")
	if requestedModelID != "" {
		for _, m := range models {
			if m.Name == "models/"+requestedModelID {
				_ = json.NewEncoder(w).Encode(m)
				return
			}
		}
		writeAPIError(w, http.StatusNotFound, "Model not found: "+requestedModelID)
		return
	}

	_ = json.NewEncoder(w).Encode(geminiModelsListResponse{Models: models})
}

// supportedModels returns the sorted upstream catalog IDs this proxy serves,
// along with the full upstream catalog.
func (s *Server) supportedModels(ctx context.Context) ([]string, map[string]antigravity.AvailableModel, error) {
	data, err := s.antigravityClient.FetchAvailableModels(ctx)
	if err != nil {
		return nil, nil, err
	}
	modelIDs := make([]string, 0, len(data.Models))
	for modelID := range data.Models {
		if isSupportedModel(modelID) {
			modelIDs = append(modelIDs, modelID)
		}
	}
	sort.Strings(modelIDs)
	return modelIDs, data.Models, nil
}

// newGeminiModel builds a models.list entry. Clients such as Zed skip entries
// without positive token limits, so these come from the upstream catalog.
func newGeminiModel(modelID string, upstream antigravity.AvailableModel) geminiModel {
	return geminiModel{
		Name:                       "models/" + modelID,
		BaseModelID:                modelID,
		Version:                    modelID,
		DisplayName:                modelDisplayName(modelID, upstream),
		InputTokenLimit:            upstream.MaxTokens,
		OutputTokenLimit:           upstream.MaxOutputTokens,
		SupportedGenerationMethods: []string{"generateContent", "streamGenerateContent"},
		Thinking:                   upstream.SupportsThinking,
	}
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var resp apiErrorResponse
	resp.Type = "error"
	resp.Error.Type = "api_error"
	resp.Error.Message = message
	_ = json.NewEncoder(w).Encode(resp)
}

func newOpenAIModel(modelID string, upstream antigravity.AvailableModel, created int64) openAIModel {
	family := modelFamily(modelID)
	ownedBy := "google"
	if family == "claude" {
		ownedBy = "anthropic"
	}
	vendor := ownedBy
	if family == "gpt" {
		vendor = "openai"
	}
	return openAIModel{
		ID:                 modelID,
		Object:             "model",
		Created:            created,
		OwnedBy:            ownedBy,
		Name:               modelDisplayName(modelID, upstream),
		Vendor:             vendor,
		Version:            modelID,
		ModelPickerEnabled: true,
		Preview:            false,
	}
}

func modelDisplayName(modelID string, upstream antigravity.AvailableModel) string {
	if name := strings.TrimSpace(upstream.DisplayName); name != "" {
		return name
	}
	return modelID
}

func isSupportedModel(modelID string) bool {
	return !isRemovedModel(modelID) && isSupportedFamily(modelFamily(modelID))
}

func isRemovedModel(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	for _, removed := range []string{"claude-sonnet-4-6", "claude-opus-4-6", "gpt-oss-120b"} {
		if modelID == removed || strings.HasPrefix(modelID, removed+"-") {
			return true
		}
	}
	return false
}

func isSupportedFamily(family string) bool {
	return family == "claude" || family == "gemini" || family == "gpt" || family == "openai"
}

func modelFamily(modelID string) string {
	lower := strings.ToLower(modelID)
	if strings.Contains(lower, "claude") {
		return "claude"
	}
	if strings.Contains(lower, "gemini") {
		return "gemini"
	}
	if strings.Contains(lower, "gpt") || strings.Contains(lower, "openai") {
		return "gpt"
	}
	return "unknown"
}
