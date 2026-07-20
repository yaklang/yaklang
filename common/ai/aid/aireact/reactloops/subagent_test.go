package reactloops

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mock "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBuildForwardingEmitterForTask_StampsTaskIdAndUUID(t *testing.T) {
	var captured []*schema.AiOutputEvent
	rootEmitter := aicommon.NewEmitter("root", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	const subTaskID = "parent-phase2-sub-xxe_ssrf-abcd"
	subTask := aicommon.NewSubTaskBaseWithOptions(
		aicommon.NewStatefulTaskBase("orchestrator", "audit", nil, rootEmitter, true),
		subTaskID,
		"scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	forwarding := BuildForwardingEmitterForTask(rootEmitter, subTask)
	require.NotNil(t, forwarding)

	_, err := forwarding.EmitStatus("read_file", "running")
	require.NoError(t, err)
	require.Len(t, captured, 1)
	require.Equal(t, subTaskID, captured[0].TaskId)
	require.Equal(t, subTask.GetUUID(), captured[0].TaskUUID)
}


func TestBuildForkTaskID_StableSegment(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("parent-abc", "x", context.Background(), aicommon.NewDummyEmitter(), true)
	id := BuildForkTaskID(task, SubAgentJob{
		Order:      1,
		Identifier: "sql_injection",
	})
	require.Contains(t, id, "parent-abc-sub-sql_injection-")
}

func TestNormalizeForkConcurrency(t *testing.T) {
	require.Equal(t, 5, normalizeSubAgentConcurrency(0, 8))
	require.Equal(t, 2, normalizeSubAgentConcurrency(0, 2))
	require.Equal(t, 10, normalizeSubAgentConcurrency(99, 20))
}

func TestForkSubTaskCompletionDoesNotCancelJobCtx(t *testing.T) {
	parent := aicommon.NewStatefulTaskBase("parent-phase2", "scan", context.Background(), aicommon.NewDummyEmitter(), true)
	jobCtx, jobCancel := context.WithCancel(parent.GetContext())
	defer jobCancel()

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parent,
		"parent-phase2-sub-cmd_injection-test",
		"category scan",
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	)
	require.NotSame(t, jobCtx, subTask.GetContext())

	subTask.SetStatus(aicommon.AITaskState_Completed)

	select {
	case <-jobCtx.Done():
		t.Fatal("jobCtx must stay alive when forked sub-task completes; only defer jobCancel should end the worker scope")
	default:
	}
}




// newSubReactAICallbackProbe returns a distinct AI callback closure that increments its own
// hit counter each time it is invoked.
func newSubReactAICallbackProbe() (aicommon.AICallbackType, *int64) {
	var hits int64
	cb := func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		atomic.AddInt64(&hits, 1)
		return &aicommon.AIResponse{}, nil
	}
	return cb, &hits
}

func assertSubAgentProbeHits(t *testing.T, label string, cb aicommon.AICallbackType, hits *int64) {
	t.Helper()
	require.NotNil(t, cb, "%s callback must be present on the child invoker", label)
	before := atomic.LoadInt64(hits)
	_, err := cb(nil, nil)
	require.NoError(t, err, "%s callback must run without error", label)
	assert.Equal(t, before+1, atomic.LoadInt64(hits), "%s callback must be the one wired into the child slot", label)
}

type subAgentTestConfigInvoker struct {
	*mock.MockInvoker
	cfg *aicommon.Config
}

func newSubAgentTestConfigInvoker(ctx context.Context, cfg *aicommon.Config) *subAgentTestConfigInvoker {
	mi := mock.NewMockInvoker(ctx)
	mi.SetConfig(cfg)
	return &subAgentTestConfigInvoker{MockInvoker: mi, cfg: cfg}
}

func (c *subAgentTestConfigInvoker) GetConfig() aicommon.AICallerConfigIf { return c.cfg }

// TestBuildSubAgentInvoker_ChildEmitterForwardsAndStampsTaskId 验证子 invoker
// 的 emitter 转发到父 emitter 并打上子任务 ID。
func TestBuildSubAgentInvoker_ChildEmitterForwardsAndStampsTaskId(t *testing.T) {
	captureEmitter := aicommon.NewEmitter("parent", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEmitter(captureEmitter),
	)

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return newSubAgentTestConfigInvoker(ctx, cfg), nil
	}

	const subTaskId = "sub-agent-42"
	taskEmitter := BuildForwardingEmitter(captureEmitter, subTaskId)
	childInvoker, err := BuildSubAgentInvokerForTest(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childEmitter := childInvoker.GetConfig().GetEmitter()
	require.NotNil(t, childEmitter)
}

// TestBuildSubAgentInvoker_PassesAICallbacksToChild 验证子 invoker 继承父的
// AI callbacks（Original / QualityPriorityRaw / SpeedPriorityRaw）。
func TestBuildSubAgentInvoker_PassesAICallbacksToChild(t *testing.T) {
	origCb, origHits := newSubReactAICallbackProbe()
	qualityCb, qualityHits := newSubReactAICallbackProbe()
	speedCb, speedHits := newSubReactAICallbackProbe()

	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithAICallbacks(&aicommon.AICallbacks{
			Original:           origCb,
			QualityPriorityRaw: qualityCb,
			SpeedPriorityRaw:   speedCb,
		}),
		aicommon.WithDisableAutoSkills(true),
	)

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return newSubAgentTestConfigInvoker(ctx, cfg), nil
	}

	taskEmitter := BuildForwardingEmitter(parentCfg.GetEmitter(), "sub-agent-cb")
	childInvoker, err := BuildSubAgentInvokerForTest(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childCfg, ok := childInvoker.GetConfig().(*aicommon.Config)
	require.True(t, ok, "child invoker config must be *aicommon.Config to expose AI callbacks")
	require.NotNil(t, childCfg)

	childRaw := childCfg.GetRawAICallbacks()
	require.NotNil(t, childRaw)

	assertSubAgentProbeHits(t, "Original", childRaw.Original, origHits)
	assertSubAgentProbeHits(t, "QualityPriorityRaw", childRaw.QualityPriorityRaw, qualityHits)
	assertSubAgentProbeHits(t, "SpeedPriorityRaw", childRaw.SpeedPriorityRaw, speedHits)
}

// TestBuildSubAgentInvoker_ChildHasNoAICallbacksWhenParentHasNone 验证父无
// callbacks 时子也不 fabricate。
func TestBuildSubAgentInvoker_ChildHasNoAICallbacksWhenParentHasNone(t *testing.T) {
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
	)
	require.Nil(t, parentCfg.GetRawAICallbacks().Original)

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub", "sub", parentCfg, parentCfg)
	require.NoError(t, err)
	require.NotNil(t, fork)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return newSubAgentTestConfigInvoker(ctx, cfg), nil
	}

	taskEmitter := BuildForwardingEmitter(parentCfg.GetEmitter(), "sub-agent-noop")
	childInvoker, err := BuildSubAgentInvokerForTest(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childCfg, ok := childInvoker.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	childRaw := childCfg.GetRawAICallbacks()
	assert.Nil(t, childRaw.Original, "child must not fabricate an Original callback when the parent has none")
	assert.Nil(t, childRaw.QualityPriorityRaw, "child must not fabricate a QualityPriorityRaw callback when the parent has none")
	assert.Nil(t, childRaw.SpeedPriorityRaw, "child must not fabricate a SpeedPriorityRaw callback when the parent has none")
}

// TestBuildSubAgentInvoker_StripsTopLevelStrategies 验证子 Agent 不继承顶层
// 执行策略（plan / goal mode / dispatch）。
func TestBuildSubAgentInvoker_StripsTopLevelStrategies(t *testing.T) {
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithEnableMultiAgentMode(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
		aicommon.WithEnablePlanAndExec(true),
	)
	require.True(t, parentCfg.GetEnableGoalMode())
	require.True(t, parentCfg.GetPreferDispatchSubReactAgents())
	require.True(t, parentCfg.GetEnablePlanAndExec())

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub-strategy", "sub", parentCfg, parentCfg)
	require.NoError(t, err)

	var capturedCfg *aicommon.Config
	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		capturedCfg = cfg
		return newSubAgentTestConfigInvoker(ctx, cfg), nil
	}

	taskEmitter := BuildForwardingEmitter(parentCfg.GetEmitter(), "sub-strategy-1")
	_, err = BuildSubAgentInvokerForTest(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, capturedCfg)

	assert.False(t, capturedCfg.GetEnableGoalMode(),
		"sub agent must not inherit goal mode")
	assert.False(t, capturedCfg.GetPreferDispatchSubReactAgents(),
		"sub agent must not inherit the multi-agent dispatch preference")
	assert.False(t, capturedCfg.GetEnablePlanAndExec(),
		"sub agent must not open plans")
	assert.False(t, capturedCfg.EnableDispatchSubReactAgents,
		"sub agent must not be able to dispatch further sub agents")
}

// TestBuildSubAgentInvoker_ChildHasFreshHotPatchOptionChan 验证子 invoker 不
// 共享父的 HotPatchOptionChan。
func TestBuildSubAgentInvoker_ChildHasFreshHotPatchOptionChan(t *testing.T) {
	parentCfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithDisableAutoSkills(true),
	)
	require.NotNil(t, parentCfg.HotPatchOptionChan, "parent must have a HotPatchOptionChan")

	parentTimeline := aicommon.NewTimeline(nil, nil)
	parentTimeline.PushText(1, "parent-seed")
	fork, err := parentTimeline.ForkForTask("sub-hotpatch", "sub", parentCfg, parentCfg)
	require.NoError(t, err)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		cfg := aicommon.NewConfig(ctx, opts...)
		return newSubAgentTestConfigInvoker(ctx, cfg), nil
	}

	taskEmitter := BuildForwardingEmitter(parentCfg.GetEmitter(), "sub-hotpatch-1")
	childInvoker, err := BuildSubAgentInvokerForTest(parentCfg, fork, context.Background(), taskEmitter)
	require.NoError(t, err)
	require.NotNil(t, childInvoker)

	childCfg, ok := childInvoker.GetConfig().(*aicommon.Config)
	require.True(t, ok)
	require.NotNil(t, childCfg.HotPatchOptionChan, "child must have its own HotPatchOptionChan")
	assert.NotSame(t, parentCfg.HotPatchOptionChan, childCfg.HotPatchOptionChan,
		"child must NOT share the parent's HotPatchOptionChan")
}


