package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestRequestPlanAction_IsRegistered(t *testing.T) {
	action, ok := reactloops.GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN)
	require.True(t, ok)
	require.NotNil(t, action)
	require.Equal(t, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN, action.ActionType)
	require.False(t, action.AsyncMode)
}
