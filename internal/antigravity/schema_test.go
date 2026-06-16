package antigravity

import (
	"encoding/json"
	"testing"
)

func TestConvertSchema(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *GeminiParameterSchema
	}{
		{
			name: "basic string type",
			input: `{
				"type": "string",
				"description": "A string"
			}`,
			expected: &GeminiParameterSchema{
				Type:        "STRING",
				Description: "A string",
			},
		},
		{
			name: "type as array of strings (e.g. string and null)",
			input: `{
				"type": ["string", "null"],
				"description": "Nullable string"
			}`,
			expected: &GeminiParameterSchema{
				Type:        "STRING",
				Description: "Nullable string",
			},
		},
		{
			name: "type as array of strings with array and null",
			input: `{
				"type": ["array", "null"],
				"items": {
					"type": "string"
				}
			}`,
			expected: &GeminiParameterSchema{
				Type: "ARRAY",
				Items: &GeminiParameterSchema{
					Type: "STRING",
				},
			},
		},
		{
			name: "missing type but has properties (implicit object)",
			input: `{
				"properties": {
					"foo": {
						"type": "string"
					}
				}
			}`,
			expected: &GeminiParameterSchema{
				Type: "OBJECT",
				Properties: map[string]*GeminiParameterSchema{
					"foo": {
						Type: "STRING",
					},
				},
			},
		},
		{
			name: "missing type but has items (implicit array)",
			input: `{
				"items": {
					"type": "string"
				}
			}`,
			expected: &GeminiParameterSchema{
				Type: "ARRAY",
				Items: &GeminiParameterSchema{
					Type: "STRING",
				},
			},
		},
		{
			name: "anyOf with string and null",
			input: `{
				"anyOf": [
					{"type": "string"},
					{"type": "null"}
				]
			}`,
			expected: &GeminiParameterSchema{
				Type: "STRING",
			},
		},
		{
			name: "anyOf with array priority",
			input: `{
				"anyOf": [
					{"type": "string"},
					{"type": "array", "items": {"type": "string"}}
				],
				"description": "Parent description"
			}`,
			expected: &GeminiParameterSchema{
				Type:        "ARRAY",
				Description: "Parent description",
				Items: &GeminiParameterSchema{
					Type: "STRING",
				},
			},
		},
		{
			name: "anyOf with type as array of strings",
			input: `{
				"anyOf": [
					{"type": ["string", "null"]}
				]
			}`,
			expected: &GeminiParameterSchema{
				Type: "STRING",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input map[string]interface{}
			if err := json.Unmarshal([]byte(tt.input), &input); err != nil {
				t.Fatalf("Failed to parse input json: %v", err)
			}

			actual := ConvertSchema(input)

			actualJSON, _ := json.Marshal(actual)
			expectedJSON, _ := json.Marshal(tt.expected)

			if string(actualJSON) != string(expectedJSON) {
				t.Errorf("ConvertSchema() mismatch\nExpected: %s\nActual:   %s", string(expectedJSON), string(actualJSON))
			}
		})
	}
}
