package loop_http_fuzztest

import (
	"context"
	"encoding/json"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"testing"
)

func TestModifyHTTPRequestDoesNotTreatDisabledClarificationAsApproval(t *testing.T) {
	original := "GET /original HTTP/1.1\r\nHost: example.com\r\n\r\n"
	loop := prepareHTTPFuzztestPatchLoop(t, original, false)
	handler, err := loop.GetActionHandler("modify_http_request")
	require.NoError(t, err)
	raw, err := json.Marshal(map[string]any{"@action": "modify_http_request", "modification_target": "path", "modification_reason": "test manual review", "require_manual_review": true, modifiedPacketContentField: "GET /changed HTTP/1.1\r\nHost: example.com\r\n\r\n"})
	require.NoError(t, err)
	action, err := aicommon.ExtractAction(string(raw), "modify_http_request")
	require.NoError(t, err)
	require.NoError(t, handler.ActionVerifier(loop, action))
	task := aicommon.NewStatefulTaskBase("approval-test", "review", context.Background(), loop.GetEmitter())
	op := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, op)
	require.Contains(t, getCurrentRequestRaw(loop), "/original")
	require.NotContains(t, getCurrentRequestRaw(loop), "/changed")
}
