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

func TestActionTypeResolutionError_ExplainsRequestedAvailableAndReason(t *testing.T) {
	err := actionTypeResolutionError(
		"save_evidence",
		[]string{"finish", "require_tool"},
		"a non-empty @action value did not match",
	)
	require.ErrorContains(t, err, `requested="save_evidence"`)
	require.ErrorContains(t, err, "matcher=exact registered action or alias")
	require.ErrorContains(t, err, "available_actions=[finish require_tool]")
	require.ErrorContains(t, err, "reason=a non-empty @action value did not match")
}

func TestActionTypeResolutionError_DistinguishesMissingAction(t *testing.T) {
	err := actionTypeResolutionError("", []string{"finish"}, "no non-empty @action value was found")
	require.ErrorContains(t, err, `requested="<missing>"`)
	require.ErrorContains(t, err, "reason=no non-empty @action value was found")
}

func TestRequiredRegisteredLoopActionError_ReportsRegistrySnapshot(t *testing.T) {
	_, err := requireRegisteredLoopAction("__missing_action_for_test__", "test capability enabled")
	require.Error(t, err)
	require.ErrorContains(t, err, `requested="__missing_action_for_test__"`)
	require.ErrorContains(t, err, `enabled_by="test capability enabled"`)
	require.ErrorContains(t, err, "registered_actions=")
	require.ErrorContains(t, err, "no action is registered under the requested key")
}
