package aid

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestCoordinatorUserInputEntersExactTimelineHistory(t *testing.T) {
	input := "  原始计划请求\n\t保留空白  \n"
	coordinator, err := NewCoordinatorContext(context.Background(), input)
	require.NoError(t, err)
	coordinator.GetTimeline().SetTimelineBucketByteSize(-1)
	options := aicommon.TimelinePromptOptions{PromoteUserInput: true}
	open := aicommon.BuildPromptFrozenOpenMaterialsWithOptions(coordinator.Config, options)
	require.Contains(t, open.TimelineOpen, input)
	require.Empty(t, open.PromptedUserInputHistory)
	coordinator.GetTimeline().FreezeAll()
	sealed := aicommon.BuildPromptFrozenOpenMaterialsWithOptions(coordinator.Config, options)
	require.Contains(t, sealed.PromptedUserInputHistory, input)
	require.NotContains(t, sealed.TimelineFrozen, input)
	require.NotContains(t, sealed.TimelineOpen, input)
	raw, err := aicommon.MarshalTimeline(coordinator.GetTimeline())
	require.NoError(t, err)
	restored, err := aicommon.UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, sealed.PromptedUserInputHistory, aicommon.RenderTimelineFrozenOpenWithOptions(restored, options).PromptedUserInputHistory)
}
