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
		{"gemini-3.8-flash", "Gemini 3.8 Flash"},
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
	wantMCPModels := []struct {
		id   string
		name string
	}{
		{"claude-opus-5-5", "claude-opus-5-5"},
		{"claude-sonnet-5-5", "Claude Sonnet 5.5"},
		{"gemini-3.8-flash-low", "Gemini 3.8 Flash (Low)"},
	}
	mcpListing, err := srv.mcpAskGeminiModels(context.Background(), askGeminiModelsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if mcpListing.DefaultModel != "" || len(mcpListing.Models) != len(wantMCPModels) {
		t.Fatalf("unexpected MCP model list: %+v", mcpListing)
	}
	for i, model := range mcpListing.Models {
		if model.ID != wantMCPModels[i].id || model.DisplayName != wantMCPModels[i].name {
			t.Errorf("MCP model %d = (%q, %q), want (%q, %q)", i, model.ID, model.DisplayName, wantMCPModels[i].id, wantMCPModels[i].name)
		}
	}
	cached, err := srv.antigravityClient.FetchAvailableModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, mutated := cached.Models["gemini-3.8-flash"]; mutated {
		t.Fatalf("FetchAvailableModels cache was mutated with synthesized base model")
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
		if rec.Code != http.StatusOK || len(listing.Models) != 3 ||
			listing.Models[0].Name != "models/claude-opus-5-5" || listing.Models[0].DisplayName != "claude-opus-5-5" ||
			listing.Models[1].Name != "models/gemini-3.8-flash" || listing.Models[1].DisplayName != "Gemini 3.8 Flash" ||
			listing.Models[2].Name != "models/gemini-3.8-flash-low" || listing.Models[2].DisplayName != "Gemini 3.8 Flash (Low)" {
			t.Fatalf("%s: unexpected listing: HTTP %d: %s", path, rec.Code, rec.Body.String())
		}
		for _, idx := range []int{1, 2} {
			if gemini := listing.Models[idx]; gemini.InputTokenLimit != 1048576 || gemini.OutputTokenLimit != 65536 || !gemini.Thinking {
				t.Errorf("%s: upstream limits not reported at index %d: %+v", path, idx, gemini)
			}
		}
	}

	if rec := get("/v1beta/models/gemini-3.8-flash"); rec.Code != http.StatusOK {
		t.Errorf("synthesized models.get returned HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get("/v1beta/models/gemini-3.8-flash-low"); rec.Code != http.StatusOK {
		t.Errorf("models.get returned HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get("/v1beta/models/claude-opus-4-6-thinking"); rec.Code != http.StatusNotFound {
		t.Errorf("removed model returned HTTP %d", rec.Code)
	}
}

func TestSynthesizeBareGeminiFamilyModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{
			"gemini-4-carbon-low":{"displayName":"Gemini 4 Carbon (Low)","maxTokens":1000,"maxOutputTokens":100,"supportsThinking":true},
			"gemini-4-carbon-medium":{"displayName":"Gemini 4 Carbon (Medium)","maxTokens":1048576,"maxOutputTokens":65536,"supportsThinking":true},
			"gemini-4-carbon-high":{"displayName":"Gemini 4 Carbon (High)","maxTokens":2000,"maxOutputTokens":200,"supportsThinking":true},
			"gemini-4-hydrogen-xhigh":{"displayName":"Gemini 4 Hydrogen (XHigh)","maxTokens":1048576,"maxOutputTokens":65536,"supportsThinking":true},
			"gemini-pro-orchestrator":{"displayName":"Orchestrator (Carbon)"},
			"gemini-pro-orchestrator-argon":{"displayName":"Orchestrator (Argon)"},
			"claude-opus-5-5-low":{"displayName":"Claude Opus 5.5 (Low)"},
			"claude-opus-5-5-medium":{"displayName":"Claude Opus 5.5 (Medium)"},
			"claude-opus-5-5-high":{"displayName":"Claude Opus 5.5 (High)"},
			"gpt-oss-120b-medium":{"displayName":"GPT-OSS 120B (Medium)"}
		}}`))
	}))
	defer upstream.Close()
	previousEndpoints := antigravity.Endpoints
	antigravity.Endpoints = []string{upstream.URL}
	t.Cleanup(func() { antigravity.Endpoints = previousEndpoints })
	srv := newMCPTestServer(t)

	rec := httptest.NewRecorder()
	srv.modelsHandler(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/models failed: HTTP %d: %s", rec.Code, rec.Body.String())
	}
	var listing openAIModelsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}

	got := make(map[string]openAIModel, len(listing.Data))
	for _, m := range listing.Data {
		got[m.ID] = m
	}

	for _, id := range []string{"gemini-4-carbon", "gemini-4-hydrogen", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon"} {
		if _, ok := got[id]; !ok {
			t.Fatalf("expected %q in model listing: %+v", id, listing.Data)
		}
	}
	if got["gemini-4-carbon"].Name != "Gemini 4 Carbon" {
		t.Errorf("gemini-4-carbon Name = %q, want %q", got["gemini-4-carbon"].Name, "Gemini 4 Carbon")
	}
	if got["gemini-4-hydrogen"].Name != "Gemini 4 Hydrogen" {
		t.Errorf("gemini-4-hydrogen Name = %q, want %q", got["gemini-4-hydrogen"].Name, "Gemini 4 Hydrogen")
	}
	for _, unexpected := range []string{"claude-opus-5-5", "gpt-oss-120b"} {
		if _, ok := got[unexpected]; ok {
			t.Errorf("did not expect synthesized model %q", unexpected)
		}
	}

	singleRec := httptest.NewRecorder()
	srv.modelsHandler(singleRec, httptest.NewRequest(http.MethodGet, "/v1/models/gemini-4-carbon", nil))
	if singleRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/models/gemini-4-carbon returned HTTP %d: %s", singleRec.Code, singleRec.Body.String())
	}

	geminiRec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-4-carbon", nil)
	srv.geminiModelsHandler(geminiRec, req)
	if geminiRec.Code != http.StatusOK {
		t.Fatalf("GET /v1beta/models/gemini-4-carbon returned HTTP %d: %s", geminiRec.Code, geminiRec.Body.String())
	}
	var gemModel geminiModel
	if err := json.Unmarshal(geminiRec.Body.Bytes(), &gemModel); err != nil {
		t.Fatal(err)
	}
	if gemModel.InputTokenLimit != 1048576 || gemModel.OutputTokenLimit != 65536 || !gemModel.Thinking {
		t.Errorf("gemini-4-carbon should inherit -medium metadata, got %+v", gemModel)
	}
}
