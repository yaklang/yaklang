package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func subAgentBroadcastConfig(t *testing.T) (context.Context, *aicommon.Config) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return &aicommon.AIResponse{}, nil
		}))
	t.Cleanup(func() { cfg.ReleaseTimelineSubscription(); cancel() })
	return ctx, cfg
}

func TestSubAgentTimelineBroadcastLifecycle(t *testing.T) {
	for _, mode := range []SubAgentTimelineMode{SubAgentTimelineFork, SubAgentTimelineClean} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			ctx, parent := subAgentBroadcastConfig(t)
			parent.Timeline.PushText(parent.AcquireId(), "parent private prefix")
			handle, err := buildTimelineHandle(parent, parent.Timeline, "child", "child", mode)
			require.NoError(t, err)
			t.Cleanup(handle.Release)
			require.NoError(t, parent.BroadcastTimelineText("before child config exists"))
			require.Contains(t, handle.Timeline().Dump(), "before child config exists")
			if mode == SubAgentTimelineClean {
				require.NotContains(t, handle.Timeline().Dump(), "parent private prefix")
			}
			opts := append(snapshotSubAgentConfig(parent, SubAgentContextTaskOnly),
				aicommon.WithTimeline(handle.Timeline()), aicommon.WithContext(ctx))
			child := aicommon.NewConfig(ctx, opts...)
			t.Cleanup(child.ReleaseTimelineSubscription)
			require.NotSame(t, parent.SessionPromptState, child.SessionPromptState)
			require.Empty(t, child.PersistentSessionId)
			require.Same(t, parent.TimelineBroadcaster, child.TimelineBroadcaster)
			require.NoError(t, child.BroadcastTimelineText("child discovery"))
			require.Contains(t, parent.Timeline.Dump(), "child discovery")
			child.ReleaseTimelineSubscription()
			handle.Release()
			require.NoError(t, parent.BroadcastTimelineText("after child release"))
			require.NotContains(t, child.Timeline.Dump(), "after child release")
		})
	}
}

func TestPrepareForkedSubAgentBroadcastRelease(t *testing.T) {
	ctx, cfg := subAgentBroadcastConfig(t)
	parent := newSubAgentTestConfigInvoker(ctx, cfg)
	task := aicommon.NewStatefulTaskBase("parent", "parent", ctx, cfg.GetEmitter(), true)
	original := aicommon.AIRuntimeInvokerGetter
	t.Cleanup(func() { aicommon.AIRuntimeInvokerGetter = original })
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		return newSubAgentTestConfigInvoker(ctx, aicommon.NewConfig(ctx, opts...)), nil
	}
	child, _, fork, release, err := PrepareForkedSubAgent(parent, task, SubAgentJob{Identifier: "broadcast", Goal: "fixture"})
	require.NoError(t, err)
	t.Cleanup(release)
	require.NoError(t, cfg.BroadcastTimelineText("while child is active"))
	require.Contains(t, fork.Branch.Dump(), "while child is active")
	release() // Legacy callers own only this cancellation function.
	release()
	require.NoError(t, cfg.BroadcastTimelineText("after cancellation"))
	require.NotContains(t, fork.Branch.Dump(), "after cancellation")
	require.ErrorIs(t, child.GetConfig().(*aicommon.Config).BroadcastTimelineText("cancelled"), context.Canceled)
}
