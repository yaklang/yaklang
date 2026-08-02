package loop_http_fuzztest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestGenerateAndSendPacketActionSchemaDeclaresTargetPurpose(t *testing.T) {
	loop := newHTTPFuzztestLoopForPatchTest(t)
	action, err := loop.GetActionHandler("generate_and_send_packet")
	require.NoError(t, err)

	options := make([]any, 0, len(action.Options))
	for _, option := range action.Options {
		options = append(options, option)
	}

	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(aitool.NewObjectSchema(options...)), &schema))
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "action schema must expose object properties")
	require.Contains(t, properties, "target_purpose",
		"the verifier requires target_purpose, so the model-facing schema must declare it")

	required, ok := schema["required"].([]any)
	require.True(t, ok, "action schema must expose required fields")
	require.Contains(t, required, "target_purpose")
}
