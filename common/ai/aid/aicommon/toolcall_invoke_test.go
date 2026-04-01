package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func TestHandleHTTPFlowMessage(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("json-httpflow", `{"runtime_id":"runtime-123","hidden_index":"flow-uuid-123","url":"http://example.com/full-data-should-be-ignored"}`)

	flow, err := handleHTTPFlowMessage(exec)
	require.NoError(t, err)
	require.NotNil(t, flow)
	require.Equal(t, "runtime-123", flow.RuntimeId)
	require.Equal(t, "flow-uuid-123", flow.HiddenIndex)
}

func TestHandleHTTPFlowMessage_IgnoreOtherLevels(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("json-risk", `{"runtime_id":"runtime-123","hidden_index":"flow-uuid-123"}`)

	flow, err := handleHTTPFlowMessage(exec)
	require.Error(t, err)
	require.Nil(t, flow)
}
