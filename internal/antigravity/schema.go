package antigravity

import "strings"

func extractTypeString(typeVal interface{}) string {
	if t, ok := typeVal.(string); ok {
		return t
	}
	if tArr, ok := typeVal.([]interface{}); ok {
		for _, tVal := range tArr {
			if tStr, ok := tVal.(string); ok && tStr != "null" {
				return tStr
			}
		}
	}
	return ""
}

// ConvertSchema recursively converts a generic map representing a JSON schema
// into the strongly-typed GeminiParameterSchema struct, only mapping supported fields.
func ConvertSchema(input map[string]interface{}) *GeminiParameterSchema {
	if input == nil {
		return nil
	}

	// Handle complex schemas with anyOf or oneOf by prioritizing the array definition.
	var subSchemas []interface{}
	if anyOf, ok := input["anyOf"].([]interface{}); ok {
		subSchemas = anyOf
	} else if oneOf, ok := input["oneOf"].([]interface{}); ok {
		subSchemas = oneOf
	}

	if subSchemas != nil {
		var fallbackSchema map[string]interface{}
		for _, subSchema := range subSchemas {
			if subSchemaMap, ok := subSchema.(map[string]interface{}); ok {
				t := extractTypeString(subSchemaMap["type"])
				if t == "array" {
					// Found the preferred array schema, convert it.
					// We also merge the description from the parent level.
					if parentDesc, ok := input["description"].(string); ok {
						subSchemaMap["description"] = parentDesc
					}
					return ConvertSchema(subSchemaMap)
				}
				if t != "" && t != "null" && fallbackSchema == nil {
					fallbackSchema = subSchemaMap
				}
			}
		}
		if fallbackSchema != nil {
			if parentDesc, ok := input["description"].(string); ok {
				fallbackSchema["description"] = parentDesc
			}
			return ConvertSchema(fallbackSchema)
		}
	}

	output := &GeminiParameterSchema{}
	if t := extractTypeString(input["type"]); t != "" {
		output.Type = strings.ToUpper(t)
	}

	if d, ok := input["description"].(string); ok {
		output.Description = d
	}

	if r, ok := input["required"].([]interface{}); ok {
		for _, v := range r {
			if s, ok := v.(string); ok {
				output.Required = append(output.Required, s)
			}
		}
	}

	if e, ok := input["enum"].([]interface{}); ok {
		for _, v := range e {
			if s, ok := v.(string); ok {
				output.Enum = append(output.Enum, s)
			}
		}
	}

	if p, ok := input["properties"].(map[string]interface{}); ok {
		output.Properties = make(map[string]*GeminiParameterSchema)
		for k, v := range p {
			if vMap, ok := v.(map[string]interface{}); ok {
				output.Properties[k] = ConvertSchema(vMap)
			}
		}
		if output.Type == "" {
			output.Type = "OBJECT"
		}
	}

	if i, ok := input["items"].(map[string]interface{}); ok {
		output.Items = ConvertSchema(i)
		if output.Type == "" {
			output.Type = "ARRAY"
		}
	}

	return output
}
