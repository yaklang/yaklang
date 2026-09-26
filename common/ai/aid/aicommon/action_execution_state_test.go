package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestActionExecutionStateIsPrivateToInvocation(t *testing.T) {
	a := NewSimpleAction("test", aitool.InvokeParams{"target": "model value"})
	b := NewSimpleAction("test", aitool.InvokeParams{})
	require.Nil(t, a.GetExecutionValue("target"), "model parameters cannot inject verified state")
	a.SetExecutionValue("target", "verified value")
	b.SetExecutionValue("target", "other invocation")
	require.Equal(t, "verified value", a.GetExecutionValue("target"))
	require.Equal(t, "other invocation", b.GetExecutionValue("target"))
	require.Equal(t, "model value", a.GetParams().GetString("target"))
	require.NotContains(t, a.DumpRawParams(), "verified value")
	a.SetExecutionValue("target", nil)
	require.Nil(t, a.GetExecutionValue("target"))
	require.Equal(t, "other invocation", b.GetExecutionValue("target"))
}
