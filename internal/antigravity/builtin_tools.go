package antigravity

import "strings"

type builtInKind int

const (
	builtInNone builtInKind = iota
	builtInGoogleSearch
	builtInURLContext
	builtInCodeExecution
)

// builtInToolKeys lists the Gemini tool keys that enable a built-in tool, in precedence order.
var builtInToolKeys = []struct {
	key  string
	kind builtInKind
	// keepOptions is false where the source options do not apply to the CloudCode tool.
	keepOptions bool
}{
	{"googleSearch", builtInGoogleSearch, true},
	{"google_search", builtInGoogleSearch, true},
	{"googleSearchRetrieval", builtInGoogleSearch, false},
	{"google_search_retrieval", builtInGoogleSearch, false},
	{"urlContext", builtInURLContext, true},
	{"url_context", builtInURLContext, true},
	{"codeExecution", builtInCodeExecution, true},
	{"code_execution", builtInCodeExecution, true},
}

// builtInToolNames maps OpenAI- and Anthropic-style tool types and names to built-in tools.
var builtInToolNames = map[string]builtInKind{
	"google_search":  builtInGoogleSearch,
	"googlesearch":   builtInGoogleSearch,
	"web_search":     builtInGoogleSearch,
	"websearch":      builtInGoogleSearch,
	"url_context":    builtInURLContext,
	"urlcontext":     builtInURLContext,
	"code_execution": builtInCodeExecution,
	"codeexecution":  builtInCodeExecution,
}

// BuiltInToolForName returns the built-in tool an OpenAI- or Anthropic-style tool type or name refers to.
func BuiltInToolForName(name string) (Tool, bool) {
	var t Tool
	t.enableBuiltIn(builtInToolNames[strings.ToLower(name)], nil)
	return t, !t.IsEmpty()
}

func (t *Tool) enableBuiltIn(kind builtInKind, opts map[string]interface{}) {
	if kind == builtInNone {
		return
	}
	if opts == nil {
		// CloudCode enables a built-in tool by the presence of its key, so the value must be {}.
		opts = map[string]interface{}{}
	}
	switch kind {
	case builtInGoogleSearch:
		if t.GoogleSearch == nil {
			t.GoogleSearch = opts
		}
	case builtInURLContext:
		if t.URLContext == nil {
			t.URLContext = opts
		}
	case builtInCodeExecution:
		if t.CodeExecution == nil {
			t.CodeExecution = opts
		}
	}
}

// applyBuiltInToolKeys enables the built-in tools named by Gemini tool keys in item.
func (t *Tool) applyBuiltInToolKeys(item map[string]interface{}) {
	for _, k := range builtInToolKeys {
		v, ok := item[k.key]
		if !ok {
			continue
		}
		var opts map[string]interface{}
		if k.keepOptions {
			opts, _ = v.(map[string]interface{})
		}
		t.enableBuiltIn(k.kind, opts)
	}
}

// rawBuiltInTool returns the built-in tool a raw tool entry refers to, either by Gemini key,
// by type, or by name. Matching by name requires a missing schema so client-defined
// functions that happen to share a built-in name stay function declarations.
func rawBuiltInTool(item map[string]interface{}) (Tool, bool) {
	var t Tool
	t.applyBuiltInToolKeys(item)
	if !t.IsEmpty() {
		return t, true
	}
	if t, ok := BuiltInToolForName(getString(item, "type")); ok {
		return t, true
	}
	if name, _, schema := extractToolFields(item); len(schema) == 0 {
		return BuiltInToolForName(name)
	}
	return Tool{}, false
}

// pruneUnsupportedBuiltInTools removes built-in tools that CloudCode cannot serve and reports
// how many were dropped. CloudCode strips urlContext and codeExecution, and rejects googleSearch
// alongside function declarations because it ignores toolConfig.includeServerSideToolInvocations.
func pruneUnsupportedBuiltInTools(tools []Tool) ([]Tool, int) {
	hasFunctions := false
	for _, t := range tools {
		if len(t.FunctionDeclarations) > 0 {
			hasFunctions = true
			break
		}
	}

	dropped := 0
	var kept []Tool
	for _, t := range tools {
		if t.URLContext != nil {
			t.URLContext = nil
			dropped++
		}
		if t.CodeExecution != nil {
			t.CodeExecution = nil
			dropped++
		}
		if hasFunctions && t.GoogleSearch != nil {
			t.GoogleSearch = nil
			dropped++
		}
		if !t.IsEmpty() {
			kept = append(kept, t)
		}
	}
	return kept, dropped
}
