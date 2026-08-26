package server

import (
	"testing"
)

func TestIsSupportedModel(t *testing.T) {
	testCases := []struct {
		modelID   string
		supported bool
		family    string
	}{
		{"gemini-3.7-flash-high", true, "gemini"},
		{"gemini-3.6-flash-low", true, "gemini"},
		{"gemini-3.1-pro-low", true, "gemini"},
		{"gemini-pro-agent", true, "gemini"},
		{"claude-sonnet-4-6", true, "claude"},
		{"claude-opus-4-6-thinking", true, "claude"},
		{"gpt-oss-120b-medium", true, "gpt"},
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

// Upstream returns the same displayName for several distinct model IDs, so the
// listing must report the ID as the name to keep entries unique and accurate.
func TestNewOpenAIModelNameIsModelID(t *testing.T) {
	// The four IDs upstream all labels "Gemini 3.1 Flash Lite".
	collidingIDs := []string{
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-2.5-flash-thinking",
		"gemini-3.1-flash-lite",
	}

	for _, id := range collidingIDs {
		t.Run(id, func(t *testing.T) {
			if got := newOpenAIModel(id, modelFamily(id), 0).Name; got != id {
				t.Errorf("newOpenAIModel(%q).Name = %q, want %q", id, got, id)
			}
		})
	}
}

func TestNewOpenAIModelOwnedBy(t *testing.T) {
	testCases := []struct {
		modelID string
		ownedBy string
	}{
		{"claude-opus-4-6-thinking", "anthropic"},
		{"claude-sonnet-4-6", "anthropic"},
		{"gemini-3.1-flash-lite", "google"},
		{"gemini-2.5-pro", "google"},
	}

	for _, tc := range testCases {
		t.Run(tc.modelID, func(t *testing.T) {
			got := newOpenAIModel(tc.modelID, modelFamily(tc.modelID), 0).OwnedBy
			if got != tc.ownedBy {
				t.Errorf("newOpenAIModel(%q).OwnedBy = %q, want %q", tc.modelID, got, tc.ownedBy)
			}
		})
	}
}
