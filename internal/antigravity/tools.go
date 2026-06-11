package antigravity

import (
	"encoding/json"
	"strings"

	"github.com/dvcrn/antigravity-oauth-proxy/internal/logger"
)

func convertRawTools(raw json.RawMessage) ([]Tool, bool) {
	toolMaps, ok := parseToolMaps(raw)
	if !ok {
		return nil, false
	}

	stats := summarizeRawTools(toolMaps)
	fns := buildFunctionDeclarations(toolMaps)
	if len(fns) == 0 {
		if stats.rawCount > 0 {
			logger.Get().Warn().
				Int("raw_tools", stats.rawCount).
				Int("missing_input_schema", stats.missingSchema).
				Int("missing_name", stats.missingName).
				Int("custom_tools", stats.customCount).
				Msg("No function declarations built from raw tools")
		}
		return nil, true
	}

	if stats.missingSchema > 0 || stats.missingName > 0 {
		logger.Get().Warn().
			Int("raw_tools", stats.rawCount).
			Int("converted_tools", len(fns)).
			Int("missing_input_schema", stats.missingSchema).
			Int("missing_name", stats.missingName).
			Int("custom_tools", stats.customCount).
			Str("tool_names", stats.previewNames()).
			Msg("Converted raw tools with missing fields")
	} else {
		logger.Get().Debug().
			Int("raw_tools", stats.rawCount).
			Int("converted_tools", len(fns)).
			Int("custom_tools", stats.customCount).
			Str("tool_names", stats.previewNames()).
			Msg("Converted raw tools to function declarations")
	}

	return []Tool{{FunctionDeclarations: fns}}, true
}

func parseToolMaps(raw json.RawMessage) ([]map[string]interface{}, bool) {
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, true
	}

	var single map[string]interface{}
	if err := json.Unmarshal(raw, &single); err == nil {
		return []map[string]interface{}{single}, true
	}

	return nil, false
}

func buildFunctionDeclarations(items []map[string]interface{}) []FunctionDeclaration {
	if len(items) == 0 {
		return nil
	}

	var fns []FunctionDeclaration
	for _, item := range items {
		name, description, schema := extractToolFields(item)
		if name == "" {
			continue
		}

		if schema == nil {
			schema = map[string]interface{}{"type": "object"}
		}

		parameters := ConvertSchema(schema)
		if parameters == nil {
			parameters = &GeminiParameterSchema{Type: "OBJECT"}
		}

		fns = append(fns, FunctionDeclaration{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		})
	}

	return fns
}

type toolStats struct {
	rawCount      int
	missingSchema int
	missingName   int
	customCount   int
	names         []string
}

func (s toolStats) previewNames() string {
	if len(s.names) == 0 {
		return ""
	}
	limit := 6
	if len(s.names) < limit {
		limit = len(s.names)
	}
	return strings.Join(s.names[:limit], ",")
}

func summarizeRawTools(items []map[string]interface{}) toolStats {
	stats := toolStats{rawCount: len(items)}
	for _, item := range items {
		name, _, schema := extractToolFields(item)
		if name == "" {
			stats.missingName++
		} else {
			stats.names = append(stats.names, name)
		}
		if schema == nil {
			stats.missingSchema++
		}
		if _, ok := item["custom"].(map[string]interface{}); ok {
			stats.customCount++
		}
	}
	return stats
}

func extractToolFields(item map[string]interface{}) (string, string, map[string]interface{}) {
	var (
		name        string
		description string
		schema      map[string]interface{}
	)

	name = getString(item, "name")
	description = getString(item, "description")
	schema = getSchema(item)

	if custom, ok := item["custom"].(map[string]interface{}); ok {
		if name == "" {
			name = getString(custom, "name")
		}
		if description == "" {
			description = getString(custom, "description")
		}
		if schema == nil {
			schema = getSchema(custom)
		}
	}

	if fn, ok := item["function"].(map[string]interface{}); ok {
		if name == "" {
			name = getString(fn, "name")
		}
		if description == "" {
			description = getString(fn, "description")
		}
		if schema == nil {
			schema = getSchema(fn)
		}
	}

	return sanitizeToolName(name), description, schema
}

func getSchema(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}
	if schema, ok := source["input_schema"].(map[string]interface{}); ok {
		return schema
	}
	if schema, ok := source["inputSchema"].(map[string]interface{}); ok {
		return schema
	}
	if schema, ok := source["parameters"].(map[string]interface{}); ok {
		return schema
	}
	return nil
}

func getString(source map[string]interface{}, key string) string {
	if source == nil {
		return ""
	}
	if v, ok := source[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func sanitizeToolName(name string) string {
	if name == "" {
		return ""
	}
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		if r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)

	if len(name) > 64 {
		name = name[:64]
	}

	return name
}
