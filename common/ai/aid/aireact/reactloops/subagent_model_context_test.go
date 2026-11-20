package reactloops

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Use the same submission snapshot and child construction as dispatched agents;
// only the final runtime constructor is replaced, so no agent task is launched.
func newModelContextTestSubAgent(t *testing.T, parent *aicommon.Config, mode string) *aicommon.Config {
	t.Helper()
	frozen := snapshotSubAgentConfig(parent, mode)
	timelineMode := SubAgentTimelineFork
	if mode == SubAgentContextTaskOnly {
		timelineMode = SubAgentTimelineClean
	}
	handle, err := buildTimelineHandle(parent, parent.Timeline, uuid.NewString(), "context child", timelineMode)
	require.NoError(t, err)
	t.Cleanup(handle.Release)
	original := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = original }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		return newSubAgentTestConfigInvoker(ctx, aicommon.NewConfig(ctx, opts...)), nil
	}
	child, err := buildSubAgentInvoker(parent, handle, parent.GetContext(), aicommon.NewDummyEmitter(), frozen)
	require.NoError(t, err)
	return child.GetConfig().(*aicommon.Config)
}

func TestSubAgentModelContextSettingsAndRecovery(t *testing.T) {
	for _, mode := range []string{SubAgentContextFork, SubAgentContextTaskOnly} {
		t.Run(mode, func(t *testing.T) {
			yakit.GuardConfigurationForTest(t)
			_, parent := newPromptOverflowRetryTestLoop(t)
			model := &ypb.AIModelConfig{ModelName: uuid.NewString(), Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai"}}
			registerModelContextTestConfig(t, model)
			require.NoError(t, aicommon.WithPrimaryModelContext(model)(parent))
			key := aispec.ModelContextIdentity(model)
			db := consts.GetGormProfileDatabase()
			child := newModelContextTestSubAgent(t, parent, mode)
			quality, speed := makeSchemaStabilityTestLoop(child), makeSchemaStabilityTestLoop(child)
			t.Cleanup(quality.Release)
			t.Cleanup(speed.Release)
			speed.useSpeedPriorityAI = true
			require.NotSame(t, parent.Timeline, child.Timeline)
			_, _, childKey := child.PrimaryModelContext()
			require.Equal(t, key, childKey)
			require.EqualValues(t, 500000, quality.wholePromptTokenLimit(), "an unconfigured quality child uses the model default, not the legacy auxiliary limit")
			disabled := true
			require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", model.ModelName, 250000, &disabled, key))
			quality.syncPromptOverflowTimelineLimit()
			require.EqualValues(t, 250000, quality.wholePromptTokenLimit())
			require.EqualValues(t, 250000, child.Timeline.GetTimelineContentLimit())

			overflow := &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "openai", Model: model.ModelName, ContextKey: key, MaxContextTokens: 200000}}
			require.True(t, quality.promptOverflowAutoAdjustDisabled(overflow))
			require.False(t, quality.reduceTimelineLimitForPromptOverflow(overflow, "prompt"))
			// Sharing a model with the quality loop must not opt auxiliary loops
			// into the saved threshold or disable their existing overflow recovery.
			speed.syncPromptOverflowTimelineLimit()
			require.EqualValues(t, aicommon.DefaultTimelineContentSizeLimit, speed.wholePromptTokenLimit())
			require.False(t, speed.promptOverflowAutoAdjustDisabled(overflow))
			require.True(t, speed.reduceTimelineLimitForPromptOverflow(overflow, "prompt"))
			require.EqualValues(t, 250000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)

			// A later edit is read from model storage, not frozen in the child options.
			disabled = false
			require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", model.ModelName, 400000, &disabled, key))
			quality.syncPromptOverflowTimelineLimit()
			require.EqualValues(t, 400000, child.Timeline.GetTimelineContentLimit())
			require.False(t, quality.promptOverflowAutoAdjustDisabled(overflow))
			require.True(t, quality.reduceTimelineLimitForPromptOverflow(overflow, "prompt"))
			require.EqualValues(t, 160000, quality.wholePromptTokenLimit())
			require.True(t, child.ModelContextStatus(0).Pending)
			require.False(t, parent.ModelContextStatus(0).Pending, "pending recovery belongs to the child, not the shared model settings")
			require.EqualValues(t, 400000, parent.ModelContextStatus(0).ThresholdTokens)
			require.EqualValues(t, 400000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)
			quality.confirmModelContextSuccess(&LoopResultDescriptor{snapshot: LoopResultSnapshot{Provider: "openai", Model: model.ModelName, ContextKey: key}}, 1000)
			require.False(t, child.ModelContextStatus(0).Pending)
			require.EqualValues(t, 160000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)
			require.EqualValues(t, 160000, parent.ModelContextStatus(0).ThresholdTokens, "only successful learning is shared with other runtimes")
		})
	}
}
