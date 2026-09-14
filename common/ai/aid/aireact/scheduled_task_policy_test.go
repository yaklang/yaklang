package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScheduledTaskSourceUsesScopedUnattendedPolicy(t *testing.T) {
	react, err := NewReAct(
		aicommon.WithAgreePolicy(aicommon.AgreePolicyManual),
		aicommon.WithAllowRequireForUserInteract(true),
		aicommon.WithAllowPlanUserInteract(true),
	)
	require.NoError(t, err)

	task := react.buildReTaskFromEvent(&ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "scheduled work",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{{
			Type:  aicommon.USER_INPUT_SOURCE,
			Key:   aicommon.USER_INPUT_SOURCE_KEY,
			Value: aicommon.USER_INPUT_SOURCE_SCHEDULE,
		}},
	})
	require.Equal(t, aicommon.USER_INPUT_SOURCE_SCHEDULE, task.GetInputSource())
	require.Empty(t, task.GetAttachedDatas(), "source metadata must not be exposed to the model as task context")

	restore := react.applyTaskExecutionPolicy(task)
	require.Equal(t, aicommon.AgreePolicyYOLO, react.config.AgreePolicy)
	require.False(t, react.config.AllowRequireForUserInteract)
	require.False(t, react.config.AllowPlanUserInteract)

	restore()
	require.Equal(t, aicommon.AgreePolicyManual, react.config.AgreePolicy)
	require.True(t, react.config.AllowRequireForUserInteract)
	require.True(t, react.config.AllowPlanUserInteract)
}
