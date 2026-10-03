package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFunctionCallMarshalsArgumentsAsObject(t *testing.T) {
	for _, input := range []string{
		`{"id":"call_1","name":"stats"}`,
		`{"id":"call_1","name":"stats","args":null}`,
		`{"id":"call_1","name":"stats","args":{}}`,
		`{"id":"call_1","name":"stats","args":{"limit":3}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var call FunctionCall
			require.NoError(t, json.Unmarshal([]byte(input), &call))
			encoded, err := json.Marshal(call)
			require.NoError(t, err)
			var wire map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Contains(t, wire, "args")
			var args map[string]interface{}
			require.NoError(t, json.Unmarshal(wire["args"], &args))
			require.NotNil(t, args, "tool input must be an object, including zero-parameter tools")
			if call.Args != nil {
				require.Equal(t, call.Args, args)
			} else {
				require.Empty(t, args)
			}
			require.JSONEq(t, `"call_1"`, string(wire["id"]))
			require.JSONEq(t, `"stats"`, string(wire["name"]))
		})
	}
}
