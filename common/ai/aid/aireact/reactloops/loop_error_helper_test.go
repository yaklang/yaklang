package reactloops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func newTestLoopForErrorHelper() *ReActLoop {
	return &ReActLoop{
		vars: omap.NewEmptyOrderedMap[string, any](),
	}
}

func TestErrorWithLastAIResponse_IncludesAIOutput(t *testing.T) {
	loop := newTestLoopForErrorHelper()
	loop.Set(LoopVarLastAIDecisionResponse, `{"@action":"finish_exploration"}`)

	got := ErrorWithLastAIResponse(loop, "plan loop finished without producing plan data")
	require.Error(t, got)
	require.Contains(t, got.Error(), "plan loop finished without producing plan data")
	require.Contains(t, got.Error(), `ai_output: {"@action":"finish_exploration"}`)
}

func TestErrorWithLastAIResponse_NoAIOutput(t *testing.T) {
	loop := newTestLoopForErrorHelper()

	got := ErrorWithLastAIResponse(loop, "plan loop finished without producing plan data")
	require.EqualError(t, got, "plan loop finished without producing plan data")
}

func TestErrorWithLastAIResponse_NilLoop(t *testing.T) {
	got := ErrorWithLastAIResponse(nil, "plan loop finished without producing plan data")
	require.EqualError(t, got, "plan loop finished without producing plan data")
}

func TestErrorWithLastAIResponse_EmptyMessageFallback(t *testing.T) {
	got := ErrorWithLastAIResponse(nil, "   ")
	require.EqualError(t, got, utils.Error("react loop failed").Error())
	require.True(t, strings.Contains(got.Error(), "react loop failed"))
}
