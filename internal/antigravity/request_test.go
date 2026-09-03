package antigravity

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestPrepareAntigravityRequestMatchesCLIShape(t *testing.T) {
	req := &GenerateContentRequest{
		Project: "test-project",
		Model:   "gemini-pro-agent",
		Request: GeminiInternalRequest{
			Contents: []Content{{
				Role:  "user",
				Parts: []ContentPart{{Text: "hello"}},
			}},
			SystemInstruction: &SystemInstruction{
				Role:  "user",
				Parts: []ContentPart{{Text: "client system"}},
			},
		},
	}

	prepareAntigravityRequest(req)

	if req.UserAgent != RequestUserAgent {
		t.Fatalf("UserAgent = %q, want %q", req.UserAgent, RequestUserAgent)
	}
	if req.RequestType != RequestTypeAgent {
		t.Fatalf("RequestType = %q, want %q", req.RequestType, RequestTypeAgent)
	}

	requestIDPattern := regexp.MustCompile(`^agent/[0-9a-f-]{36}/[0-9]{13}/[0-9a-f-]{36}/1$`)
	if !requestIDPattern.MatchString(req.RequestID) {
		t.Fatalf("RequestID = %q, want Antigravity CLI shape", req.RequestID)
	}

	if req.Request.SystemInstruction == nil {
		t.Fatal("SystemInstruction is nil")
	}
	if req.Request.SystemInstruction.Role != "user" {
		t.Fatalf("SystemInstruction.Role = %q, want user", req.Request.SystemInstruction.Role)
	}
	parts := req.Request.SystemInstruction.Parts
	if len(parts) != 2 {
		t.Fatalf("SystemInstruction parts = %d, want 2", len(parts))
	}
	if !strings.Contains(parts[0].Text, "<identity>") || !strings.Contains(parts[0].Text, "You are Antigravity") {
		t.Fatalf("first system part does not contain Antigravity identity")
	}
	if strings.Contains(parts[0].Text, "Please ignore the following [ignore]") {
		t.Fatalf("first system part contains legacy ignore injection")
	}
	if parts[1].Text != "client system" {
		t.Fatalf("existing system part = %q, want client system", parts[1].Text)
	}
}

func TestPrepareAntigravityRequestDefaultsThinkingConfig(t *testing.T) {
	req := &GenerateContentRequest{
		Model: "gemini-3.1-pro-low",
		Request: GeminiInternalRequest{
			Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
		},
	}

	prepareAntigravityRequest(req)

	if req.Request.GenerationConfig == nil || req.Request.GenerationConfig.ThinkingConfig == nil {
		t.Fatal("ThinkingConfig was not defaulted")
	}
	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if thinkingConfig.IncludeThoughts == nil || !*thinkingConfig.IncludeThoughts {
		t.Fatalf("IncludeThoughts = %v, want true", thinkingConfig.IncludeThoughts)
	}
	if thinkingConfig.ThinkingBudget == nil || *thinkingConfig.ThinkingBudget != 10001 {
		t.Fatalf("ThinkingBudget = %v, want 10001", thinkingConfig.ThinkingBudget)
	}
}

func TestPrepareAntigravityRequestClearsThinkingLevelForEncodedModel(t *testing.T) {
	testModels := []string{
		"gemini-3.1-pro-high",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-low",
	}

	for _, model := range testModels {
		t.Run(model, func(t *testing.T) {
			includeThoughts := false
			req := &GenerateContentRequest{
				Model: model,
				Request: GeminiInternalRequest{
					Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
					GenerationConfig: &GeminiGenerationConfig{
						ThinkingConfig: &ThinkingConfig{
							IncludeThoughts: &includeThoughts,
							ThinkingLevel:   "LOW",
						},
					},
				},
			}

			prepareAntigravityRequest(req)

			thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
			if thinkingConfig.ThinkingLevel != "" {
				t.Fatalf("ThinkingLevel = %q, want empty", thinkingConfig.ThinkingLevel)
			}
			if thinkingConfig.IncludeThoughts == nil || *thinkingConfig.IncludeThoughts {
				t.Fatalf("IncludeThoughts = %v, want false", thinkingConfig.IncludeThoughts)
			}
			if thinkingConfig.ThinkingBudget == nil || *thinkingConfig.ThinkingBudget != 10001 {
				t.Fatalf("ThinkingBudget = %v, want 10001", thinkingConfig.ThinkingBudget)
			}
		})
	}
}

func TestPrepareAntigravityRequestStripsThinkingConfigForGptOss(t *testing.T) {
	req := &GenerateContentRequest{
		Model: "gpt-oss-120b-medium",
		Request: GeminiInternalRequest{
			Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
		},
	}

	prepareAntigravityRequest(req)

	if req.Request.GenerationConfig != nil && req.Request.GenerationConfig.ThinkingConfig != nil {
		t.Fatalf("ThinkingConfig should be nil for gpt-oss models")
	}
}

func TestPrepareAntigravityRequestAppliesGemini37ThinkingPreset(t *testing.T) {
	req := &GenerateContentRequest{
		Model: "gemini-3.7-flash-high",
		Request: GeminiInternalRequest{
			Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
		},
	}

	prepareAntigravityRequest(req)

	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if thinkingConfig.ThinkingLevel != "high" {
		t.Fatalf("ThinkingLevel = %q, want %q", thinkingConfig.ThinkingLevel, "high")
	}
}

func TestPrepareAntigravityRequestAppliesGemini38ThinkingPreset(t *testing.T) {
	req := &GenerateContentRequest{
		Model: "gemini-3.8-flash-high",
		Request: GeminiInternalRequest{
			Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
		},
	}

	prepareAntigravityRequest(req)

	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if thinkingConfig.ThinkingLevel != "high" {
		t.Fatalf("ThinkingLevel = %q, want %q", thinkingConfig.ThinkingLevel, "high")
	}
}

func TestPrepareAntigravityRequestPreservesThinkingConfig(t *testing.T) {
	includeThoughts := false
	thinkingBudget := 123
	req := &GenerateContentRequest{
		Model: "gemini-3.1-pro-low",
		Request: GeminiInternalRequest{
			Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
			GenerationConfig: &GeminiGenerationConfig{
				ThinkingConfig: &ThinkingConfig{
					IncludeThoughts: &includeThoughts,
					ThinkingBudget:  &thinkingBudget,
				},
			},
		},
	}

	prepareAntigravityRequest(req)

	thinkingConfig := req.Request.GenerationConfig.ThinkingConfig
	if thinkingConfig.IncludeThoughts == nil || *thinkingConfig.IncludeThoughts {
		t.Fatalf("IncludeThoughts = %v, want false", thinkingConfig.IncludeThoughts)
	}
	if thinkingConfig.ThinkingBudget == nil || *thinkingConfig.ThinkingBudget != 123 {
		t.Fatalf("ThinkingBudget = %v, want 123", thinkingConfig.ThinkingBudget)
	}
}

func TestPrepareAntigravityRequestClampsMaxOutputTokens(t *testing.T) {
	testCases := []struct {
		model      string
		maxTokens  int
		wantTokens int
	}{
		{"gemini-3.1-flash-lite", 65536, 32768},
		{"gemini-3.1-flash-lite", 100000, 32768},
		{"gemini-3.1-flash-lite", 4096, 4096},
		{"gemini-3.1-pro-low", 65536, 32768},
		{"gemini-2.5-flash", 65536, 32768},
		{"gemini-3-flash-agent", 65536, 65536},
		{"gemini-3.7-flash-high", 65536, 65536},
		{"gemini-3.8-flash-high", 65536, 65536},
		{"gemini-3-flash", 65536, 65536},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			req := &GenerateContentRequest{
				Model: tc.model,
				Request: GeminiInternalRequest{
					Contents: []Content{{Role: "user", Parts: []ContentPart{{Text: "hello"}}}},
					GenerationConfig: &GeminiGenerationConfig{
						MaxOutputTokens: tc.maxTokens,
					},
				},
			}

			prepareAntigravityRequest(req)

			if req.Request.GenerationConfig.MaxOutputTokens != tc.wantTokens {
				t.Fatalf("MaxOutputTokens = %d, want %d", req.Request.GenerationConfig.MaxOutputTokens, tc.wantTokens)
			}
		})
	}
}

func TestLoadCodeAssistResponseParsesPaidTier(t *testing.T) {
	body := []byte(`{
		"currentTier":{"id":"free-tier","name":"Antigravity","description":"Gemini-powered code suggestions"},
		"allowedTiers":[{"id":"free-tier","name":"Antigravity","isDefault":true}],
		"cloudaicompanionProject":"aesthetic-container-3v00q",
		"gcpManaged":false,
		"upgradeSubscriptionUri":"https://codeassist.google.com/upgrade",
		"paidTier":{
			"id":"g1-pro-tier",
			"name":"Google AI Pro",
			"description":"Google AI Pro",
			"upgradeSubscriptionUri":"https://antigravity.google/g1-upgrade",
			"upgradeSubscriptionText":"You can upgrade to a Google AI Ultra plan.",
			"availableCredits":[{"creditType":"GOOGLE_ONE_AI","creditAmount":"1000","minimumCreditAmountForUsage":"50"}]
		}
	}`)

	var resp LoadCodeAssistResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.UpgradeSubscriptionURI != "https://codeassist.google.com/upgrade" {
		t.Fatalf("UpgradeSubscriptionURI = %q", resp.UpgradeSubscriptionURI)
	}
	if resp.PaidTier == nil {
		t.Fatal("PaidTier is nil")
	}
	if resp.PaidTier.ID != "g1-pro-tier" {
		t.Fatalf("PaidTier.ID = %q", resp.PaidTier.ID)
	}
	if len(resp.PaidTier.AvailableCredits) != 1 || resp.PaidTier.AvailableCredits[0].CreditAmount != "1000" {
		t.Fatalf("AvailableCredits = %#v", resp.PaidTier.AvailableCredits)
	}
}

func TestApplyHeadersMatchesAntigravityCLI(t *testing.T) {
	header := http.Header{}
	ApplyHeaders(header, "token", "application/json")

	if got := header.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := header.Get("User-Agent"); !strings.HasPrefix(got, "antigravity/cli/1.1.13 (aidev_client;") {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := header.Get("X-Goog-Api-Client"); got != "" {
		t.Fatalf("X-Goog-Api-Client = %q, want empty", got)
	}
	if got := header.Get("Client-Metadata"); got != "" {
		t.Fatalf("Client-Metadata = %q, want empty", got)
	}
	if got := header.Get("Accept"); got != "" {
		t.Fatalf("Accept = %q, want empty", got)
	}
}
