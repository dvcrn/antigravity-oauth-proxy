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
