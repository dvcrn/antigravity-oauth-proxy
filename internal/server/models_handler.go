package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	Name    string `json:"name,omitempty"`
}

type openAIModelsListResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
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

	data, err := s.antigravityClient.FetchAvailableModels(r.Context())
	if err != nil {
		logger.Get().Error().Err(err).Msg("Failed to fetch available models")
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// One timestamp for the whole listing: a per-model time.Now() lets entries in
	// the same response straddle a second boundary and report different values.
	created := time.Now().Unix()

	models := make([]openAIModel, 0, len(data.Models))
	for modelID := range data.Models {
		family := modelFamily(modelID)
		if !isSupportedFamily(family) {
			continue
		}
		models = append(models, newOpenAIModel(modelID, family, created))
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

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

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var resp apiErrorResponse
	resp.Type = "error"
	resp.Error.Type = "api_error"
	resp.Error.Message = message
	_ = json.NewEncoder(w).Encode(resp)
}

// newOpenAIModel builds the listing entry for a single upstream model ID.
//
// Name echoes the model ID; see AvailableModel.DisplayName for why upstream's
// label is not used.
func newOpenAIModel(modelID, family string, created int64) openAIModel {
	ownedBy := "google"
	if family == "claude" {
		ownedBy = "anthropic"
	}
	return openAIModel{
		ID:      modelID,
		Object:  "model",
		Created: created,
		OwnedBy: ownedBy,
		Name:    modelID,
	}
}

func isSupportedModel(modelID string) bool {
	return isSupportedFamily(modelFamily(modelID))
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
