package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/antigravity"
)

func TestModelListingsExcludeRemovedModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"defaultAgentModelId":"claude-opus-4-6-thinking","models":{
			"claude-sonnet-4-6":{},"claude-opus-4-6-thinking":{},"gpt-oss-120b-medium":{},
			"claude-sonnet-5-5":{"displayName":"Claude Sonnet 5.5"},"claude-opus-5-5":{},
			"gemini-3.8-flash-low":{"displayName":"Gemini 3.8 Flash (Low)"}}}`))
	}))
	defer upstream.Close()
	previousEndpoints := antigravity.Endpoints
	antigravity.Endpoints = []string{upstream.URL}
	t.Cleanup(func() { antigravity.Endpoints = previousEndpoints })
	srv := newMCPTestServer(t)

	rec := httptest.NewRecorder()
	srv.modelsHandler(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var listing openAIModelsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	wantModels := []struct {
		id   string
		name string
	}{
		{"claude-opus-5-5", "claude-opus-5-5"},
		{"claude-sonnet-5-5", "Claude Sonnet 5.5"},
		{"gemini-3.8-flash-low", "Gemini 3.8 Flash (Low)"},
	}
	if rec.Code != http.StatusOK || len(listing.Data) != len(wantModels) {
		t.Fatalf("unexpected model list: HTTP %d: %s", rec.Code, rec.Body.String())
	}
	for i, model := range listing.Data {
		if model.ID != wantModels[i].id || model.Name != wantModels[i].name {
			t.Errorf("model %d = (%q, %q), want (%q, %q)", i, model.ID, model.Name, wantModels[i].id, wantModels[i].name)
		}
	}
	for _, model := range []string{"claude-sonnet-4-6", "claude-opus-4-6-thinking", "gpt-oss-120b-medium"} {
		rec := httptest.NewRecorder()
		srv.modelsHandler(rec, httptest.NewRequest(http.MethodGet, "/v1/models/"+model, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("removed model %q returned HTTP %d", model, rec.Code)
		}
	}
	mcpListing, err := srv.mcpAskGeminiModels(context.Background(), askGeminiModelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if mcpListing.DefaultModel != "" || len(mcpListing.Models) != len(wantModels) {
		t.Fatalf("unexpected MCP model list: %+v", mcpListing)
	}
	for i, model := range mcpListing.Models {
		if model.ID != wantModels[i].id || model.DisplayName != wantModels[i].name {
			t.Errorf("MCP model %d = (%q, %q), want (%q, %q)", i, model.ID, model.DisplayName, wantModels[i].id, wantModels[i].name)
		}
	}
}

func TestIsSupportedModel(t *testing.T) {
	testCases := []struct {
		modelID   string
		supported bool
		family    string
	}{
		{"gemini-4-argon-low", true, "gemini"},
		{"gemini-4-argon-medium", true, "gemini"},
		{"gemini-4-argon-high", true, "gemini"},
		{"gemini-4-carbon-low", true, "gemini"},
		{"gemini-4-carbon-medium", true, "gemini"},
		{"gemini-4-carbon-high", true, "gemini"},
		{"gemini-4-hydrogen-low", true, "gemini"},
		{"gemini-4-hydrogen-medium", true, "gemini"},
		{"gemini-4-hydrogen-high", true, "gemini"},
		{"gemini-4-hydrogen-xhigh", true, "gemini"},
		{"gemini-pro-orchestrator", true, "gemini"},
		{"gemini-pro-orchestrator-argon", true, "gemini"},
		{"gemini-flash-worker-agy", true, "gemini"},
		{"gemini-3.8-flash-high", true, "gemini"},
		{"claude-sonnet-4-6", false, "claude"},
		{"claude-sonnet-4-6-thinking", false, "claude"},
		{"claude-opus-4-6", false, "claude"},
		{"claude-opus-4-6-thinking", false, "claude"},
		{"gpt-oss-120b", false, "gpt"},
		{"gpt-oss-120b-medium", false, "gpt"},
		{" CLAUDE-OPUS-4-6-THINKING ", false, "claude"},
		{"claude-sonnet-5-5", true, "claude"},
		{"claude-opus-5-5", true, "claude"},
		{"claude-sonnet-4-60", true, "claude"},
		{"openai-gpt-4o", true, "gpt"},
		{"chat_20706", false, "unknown"},
		{"tab_jump_flash_lite_preview", false, "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.modelID, func(t *testing.T) {
			gotFamily := modelFamily(tc.modelID)
			if gotFamily != tc.family {
				t.Errorf("modelFamily(%q) = %q, want %q", tc.modelID, gotFamily, tc.family)
			}
			gotSupported := isSupportedModel(tc.modelID)
			if gotSupported != tc.supported {
				t.Errorf("isSupportedModel(%q) = %v, want %v", tc.modelID, gotSupported, tc.supported)
			}
		})
	}
}

func TestNewOpenAIModelName(t *testing.T) {
	testCases := []struct {
		name        string
		modelID     string
		displayName string
		wantName    string
	}{
		{"upstream display name", "gemini-pro-orchestrator-argon", "Orchestrator (Argon)", "Orchestrator (Argon)"},
		{"trimmed display name", "gemini-4-argon-high", "  Gemini 4 Argon (High)  ", "Gemini 4 Argon (High)"},
		{"empty display name falls back to id", "gemini-3.8-flash-tiered", "", "gemini-3.8-flash-tiered"},
		{"whitespace display name falls back to id", "gemini-3.8-flash-tiered", "   ", "gemini-3.8-flash-tiered"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := newOpenAIModel(tc.modelID, antigravity.AvailableModel{DisplayName: tc.displayName}, 0)
			if got.ID != tc.modelID || got.Name != tc.wantName || got.Version != tc.modelID {
				t.Errorf("newOpenAIModel(%q, %q) = {ID:%q Name:%q Version:%q}, want {ID:%q Name:%q Version:%q}",
					tc.modelID, tc.displayName, got.ID, got.Name, got.Version, tc.modelID, tc.wantName, tc.modelID)
			}
		})
	}
}

func TestNewOpenAIModelPickerFields(t *testing.T) {
	for _, tc := range []struct {
		id     string
		vendor string
	}{
		{"claude-sonnet-5-5-high", "anthropic"},
		{"gemini-4-argon-high", "google"},
		{"openai-gpt-4o", "openai"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			model := newOpenAIModel(tc.id, antigravity.AvailableModel{}, 0)
			if !model.ModelPickerEnabled || model.Vendor != tc.vendor || model.Version != tc.id {
				t.Errorf("model picker fields for %q: %+v", tc.id, model)
			}
		})
	}
}

func TestNewOpenAIModelOwnedBy(t *testing.T) {
	testCases := []struct {
		modelID string
		ownedBy string
	}{
		{"claude-opus-5-5", "anthropic"},
		{"claude-sonnet-5-5", "anthropic"},
		{"gemini-3.1-flash-lite", "google"},
		{"gemini-2.5-pro", "google"},
	}

	for _, tc := range testCases {
		t.Run(tc.modelID, func(t *testing.T) {
			got := newOpenAIModel(tc.modelID, antigravity.AvailableModel{}, 0).OwnedBy
			if got != tc.ownedBy {
				t.Errorf("newOpenAIModel(%q).OwnedBy = %q, want %q", tc.modelID, got, tc.ownedBy)
			}
		})
	}
}

func TestGeminiModelsRoutes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"claude-opus-4-6-thinking":{},"claude-opus-5-5":{},
			"gemini-3.8-flash-low":{"displayName":"Gemini 3.8 Flash (Low)","maxTokens":1048576,"maxOutputTokens":65536,"supportsThinking":true}}}`))
	}))
	defer upstream.Close()
	previousEndpoints := antigravity.Endpoints
	antigravity.Endpoints = []string{upstream.URL}
	t.Cleanup(func() { antigravity.Endpoints = previousEndpoints })
	srv := newMCPTestServer(t)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+testAdminKey)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	for _, path := range []string{"/v1beta/models?pageSize=1000", "/v1beta/models/?pageSize=1000"} {
		rec := get(path)
		var listing geminiModelsListResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
			t.Fatalf("%s: HTTP %d: %s", path, rec.Code, rec.Body.String())
		}
		if rec.Code != http.StatusOK || len(listing.Models) != 2 ||
			listing.Models[0].Name != "models/claude-opus-5-5" || listing.Models[0].DisplayName != "claude-opus-5-5" ||
			listing.Models[1].Name != "models/gemini-3.8-flash-low" || listing.Models[1].DisplayName != "Gemini 3.8 Flash (Low)" {
			t.Fatalf("%s: unexpected listing: HTTP %d: %s", path, rec.Code, rec.Body.String())
		}
		if gemini := listing.Models[1]; gemini.InputTokenLimit != 1048576 || gemini.OutputTokenLimit != 65536 || !gemini.Thinking {
			t.Errorf("%s: upstream limits not reported: %+v", path, gemini)
		}
	}

	if rec := get("/v1beta/models/gemini-3.8-flash-low"); rec.Code != http.StatusOK {
		t.Errorf("models.get returned HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get("/v1beta/models/claude-opus-4-6-thinking"); rec.Code != http.StatusNotFound {
		t.Errorf("removed model returned HTTP %d", rec.Code)
	}
}
