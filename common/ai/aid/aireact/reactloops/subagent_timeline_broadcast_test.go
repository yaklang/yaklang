package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func subAgentBroadcastConfig(t *testing.T) (context.Context, *aicommon.Config) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return &aicommon.AIResponse{}, nil
		}))
	t.Cleanup(func() { cfg.UnregisterTimeline(); cancel() })
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
			t.Cleanup(child.UnregisterTimeline)
			require.NotSame(t, parent.SessionPromptState, child.SessionPromptState)
			require.Empty(t, child.PersistentSessionId)
			require.Same(t, parent.TimelineBroadcaster, child.TimelineBroadcaster)
			require.NoError(t, child.BroadcastTimelineText("child discovery"))
			require.Contains(t, parent.Timeline.Dump(), "child discovery")
			child.UnregisterTimeline()
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

func TestSubAgentReportedRiskTimelineContextPolicy(t *testing.T) {
	for _, mode := range []SubAgentTimelineMode{SubAgentTimelineFork, SubAgentTimelineClean} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			ctx, parent := subAgentBroadcastConfig(t)
			require.True(t, parent.AppendReportedRisk(&schema.Risk{Url: "https://example.com/old-risk", RiskType: "xss", Title: "old-risk"}))
			parent.Timeline.FreezeAll()
			handle, err := buildTimelineHandle(parent, parent.Timeline, "child", "child", mode)
			require.NoError(t, err)
			t.Cleanup(handle.Release)
			contextMode := SubAgentContextFork
			if mode == SubAgentTimelineClean {
				contextMode = SubAgentContextTaskOnly
			}
			opts := append(snapshotSubAgentConfig(parent, contextMode), aicommon.WithTimeline(handle.Timeline()), aicommon.WithContext(ctx))
			child := aicommon.NewConfig(ctx, opts...)
			t.Cleanup(child.UnregisterTimeline)
			view := aicommon.RenderTimelineFrozenOpen(child.Timeline)
			if mode == SubAgentTimelineClean {
				require.Empty(t, view.PromptText())
				require.Empty(t, aicommon.BuildPromptFrozenOpenMaterials(child).PromotedReportedRisks)
			} else {
				require.Contains(t, view.PromotedReportedRisks, "old-risk")
			}
			require.True(t, parent.AppendReportedRisk(&schema.Risk{Url: "https://example.com/new-risk", RiskType: "xss", Title: "new-risk"}))
			require.Contains(t, aicommon.RenderTimelineFrozenOpen(child.Timeline).Open, "new-risk")
			child.Timeline.FreezeAll()
			updated := aicommon.RenderTimelineFrozenOpen(child.Timeline).PromotedReportedRisks
			require.Contains(t, updated, "new-risk")
			if mode == SubAgentTimelineClean {
				require.NotContains(t, updated, "old-risk")
			}
		})
	}
}
