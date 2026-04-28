package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func parseActionForInference(t *testing.T, raw string) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractActionFromStream(context.Background(), strings.NewReader(raw), "object")
	require.NoError(t, err)
	action.WaitParse(context.Background())
	action.WaitStream(context.Background())
	return action
}

func TestInferActionTypeFromPayload_UsesNestedPayloadWhenTypeMissing(t *testing.T) {
	action := parseActionForInference(t, `{"@action":"object","next_action":{"answer_payload":"hello"}}`)
	require.Equal(t, "directly_answer", inferActionTypeFromPayload(action, ""))
}

func TestInferActionTypeFromPayload_UsesPlanPayloadWhenTypeMissing(t *testing.T) {
	action := parseActionForInference(t, `{"@action":"object","next_action":{"plan_request_payload":"inspect project auth flow"}}`)
	require.Equal(t, "request_plan_and_execution", inferActionTypeFromPayload(action, ""))
}

func TestInferActionTypeFromPayload_UsesFinalAnswerTagAsFallback(t *testing.T) {
	action := aicommon.NewSimpleAction("", nil)
	require.Equal(t, "directly_answer", inferActionTypeFromPayload(action, "## final answer"))
}
