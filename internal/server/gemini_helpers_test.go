package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeModelName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "gemini-4-argon-high",
			input:    "gemini-4-argon-high",
			expected: "gemini-4-argon-high",
		},
		{
			name:     "gemini-pro-orchestrator-argon",
			input:    "gemini-pro-orchestrator-argon",
			expected: "gemini-pro-orchestrator-argon",
		},
		{
			name:     "gemini-3-flash",
			input:    "gemini-3-flash",
			expected: "gemini-3-flash",
		},
		{
			name:     "gemini-2.5-pro",
			input:    "gemini-2.5-pro",
			expected: "gemini-2.5-pro",
		},
		{
			name:     "gemini-2.5-flash",
			input:    "gemini-2.5-flash",
			expected: "gemini-2.5-flash",
		},
		{
			name:     "unknown model passes through",
			input:    "unknown-model",
			expected: "unknown-model",
		},
		{
			name:     "custom model variant",
			input:    "custom-variant-123",
			expected: "custom-variant-123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			normalized := normalizeModelName(tc.input)
			if normalized != tc.expected {
				t.Errorf("Expected model name %s, but got %s", tc.expected, normalized)
			}
		})
	}
}

func TestNormalizeCitationMetadata(t *testing.T) {
	t.Run("mirrors citations onto citationSources", func(t *testing.T) {
		response := map[string]interface{}{
			"candidates": []interface{}{
				map[string]interface{}{
					"citationMetadata": map[string]interface{}{
						"citations": []interface{}{
							map[string]interface{}{
								"startIndex": float64(12),
								"endIndex":   float64(297),
								"uri":        "https://github.com/getsentry/sentry-elixir",
							},
						},
					},
				},
			},
		}

		normalizeCitationMetadata(response)

		candidate := response["candidates"].([]interface{})[0].(map[string]interface{})
		metadata := candidate["citationMetadata"].(map[string]interface{})

		sources, ok := metadata["citationSources"].([]interface{})
		if !ok {
			t.Fatalf("expected citationSources to be present, got %#v", metadata)
		}
		if len(sources) != 1 {
			t.Fatalf("expected 1 citation source, got %d", len(sources))
		}
		if _, ok := metadata["citations"]; !ok {
			t.Error("expected original citations field to be preserved")
		}
	})

	t.Run("leaves existing citationSources untouched", func(t *testing.T) {
		response := map[string]interface{}{
			"candidates": []interface{}{
				map[string]interface{}{
					"citationMetadata": map[string]interface{}{
						"citations":       []interface{}{"from-citations"},
						"citationSources": []interface{}{"original"},
					},
				},
			},
		}

		normalizeCitationMetadata(response)

		candidate := response["candidates"].([]interface{})[0].(map[string]interface{})
		metadata := candidate["citationMetadata"].(map[string]interface{})
		sources := metadata["citationSources"].([]interface{})
		if len(sources) != 1 || sources[0] != "original" {
			t.Errorf("expected existing citationSources to be preserved, got %#v", sources)
		}
	})

	t.Run("tolerates missing or malformed fields", func(t *testing.T) {
		cases := []map[string]interface{}{
			{},
			{"candidates": "not-a-list"},
			{"candidates": []interface{}{"not-a-map"}},
			{"candidates": []interface{}{map[string]interface{}{}}},
			{"candidates": []interface{}{map[string]interface{}{"citationMetadata": "nope"}}},
			{"candidates": []interface{}{map[string]interface{}{"citationMetadata": map[string]interface{}{}}}},
		}

		for _, response := range cases {
			normalizeCitationMetadata(response)
		}
	})
}

func TestTransformSSELineAddsCitationSources(t *testing.T) {
	line := `data: {"response":{"candidates":[{"citationMetadata":{"citations":[{"endIndex":297,"startIndex":12,"uri":"https://github.com/getsentry/sentry-elixir"}]},"content":{"parts":[{"text":"hello"}],"role":"model"}}],"modelVersion":"gemini-3.8-flash"}}`

	transformed := TransformSSELine(line)

	if !strings.HasPrefix(transformed, "data: ") {
		t.Fatalf("expected SSE data prefix, got %q", transformed)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(transformed, "data: ")), &parsed); err != nil {
		t.Fatalf("failed to parse transformed line: %v", err)
	}

	candidate := parsed["candidates"].([]interface{})[0].(map[string]interface{})
	metadata := candidate["citationMetadata"].(map[string]interface{})
	if _, ok := metadata["citationSources"]; !ok {
		t.Errorf("expected citationSources in transformed SSE line, got %#v", metadata)
	}
}
