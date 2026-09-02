package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

// registerStubRequireToolAction 注册一个最小 require_tool 动作满足
// NewReActLoop 的强制注册检查（测试进程内可能没有生产 loop 包的 init）。
func registerStubRequireToolAction(t *testing.T) {
	t.Helper()
	prev, hadPrev := GetLoopAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
	RegisterAction(&LoopAction{ActionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, Description: "stub"})
	t.Cleanup(func() {
		if hadPrev {
			RegisterAction(prev)
		}
	})
}

// minimalNewReActLoopOptions 关闭所有"有条件必需"的 action 通路（RAG/forge/
// plan/交互），只留强制 require_tool（由 registerStubRequireToolAction 提供 stub），
// 使 NewReActLoop 能在测试进程内（无生产 loop init）完成构建。
func minimalNewReActLoopOptions() []ReActLoopOption {
	return []ReActLoopOption{
		WithAllowRAG(false),
		WithAllowAIForge(false),
		WithAllowPlanAndExec(false),
		WithAllowUserInteract(false),
	}
}

// TestNewReActLoop_ConfigLevelDisablePeriodicVerification 验证 config entry
// "DisablePeriodicVerification" 在 NewReActLoop 时生效（仿 DisablePerception 模式），
// 使不带 per-loop 选项构建的子 loop（如 fast_context）也关闭周期验证。
func TestNewReActLoop_ConfigLevelDisablePeriodicVerification(t *testing.T) {
	registerStubRequireToolAction(t)
	invoker := mock.NewMockInvoker(context.Background())
	cfg := aicommon.NewConfig(context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisablePeriodicVerification(true),
	)
	invoker.SetConfig(cfg)

	loop, err := NewReActLoop("config_level_periodic_verification_loop", invoker, minimalNewReActLoopOptions()...)
	require.NoError(t, err)
	require.NotNil(t, loop)
	require.True(t, loop.DisablePeriodicVerification,
		"config entry DisablePeriodicVerification must disable periodic verification on the loop")
}

// TestNewReActLoop_ConfigLevelDisablePeriodicVerification_DefaultOff 验证默认
// 不关闭（不改变非审计场景的全局默认行为）。
func TestNewReActLoop_ConfigLevelDisablePeriodicVerification_DefaultOff(t *testing.T) {
	registerStubRequireToolAction(t)
	invoker := mock.NewMockInvoker(context.Background())
	cfg := aicommon.NewConfig(context.Background(),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
	)
	invoker.SetConfig(cfg)

	loop, err := NewReActLoop("config_level_periodic_verification_loop", invoker, minimalNewReActLoopOptions()...)
	require.NoError(t, err)
	require.NotNil(t, loop)
	require.False(t, loop.DisablePeriodicVerification)
}

// TestExecuteSubAgents_AppliesExtraLoopOpts 验证 SubAgentOptions.ExtraLoopOpts
// 不再是死参数：在 loop 构建后、执行前被应用。
func TestExecuteSubAgents_AppliesExtraLoopOpts(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	parentCfg := aicommon.NewConfig(parentCtx, aicommon.WithDisableAutoSkills(true))
	parentInvoker := mock.NewMockInvoker(parentCtx)
	parentInvoker.SetConfig(parentCfg)
	parentTask := aicommon.NewStatefulTaskBase("task-extra-loop-opts", "extra loop opts", parentCtx, nil, true)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(childCtx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		childCfg := aicommon.NewConfig(childCtx, opts...)
		child := mock.NewMockInvoker(childCtx)
		child.SetConfig(childCfg)
		return child, nil
	}

	testLoopName := "test_extra_loop_opts_loop"
	_ = RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			loop := NewMinimalReActLoop(r.GetConfig(), r)
			for _, opt := range opts {
				opt(loop)
			}
			WithInitTask(func(l *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
				op.Done()
			})(loop)
			return loop, nil
		},
	)

	results := DispatchSubAgents(parentInvoker, parentTask, []SubAgentJob{{
		Order:      1,
		Identifier: "extra-loop-opts-job",
		TaskName:   "extra loop opts",
		Goal:       "extra loop opts",
		LoopName:   testLoopName,
	}}, SubAgentOptions{
		TimelineMode: SubAgentTimelineClean,
		ExtraLoopOpts: []ReActLoopOption{WithDisablePeriodicVerification(true)},
	})
	require.Len(t, results, 1)
	require.NotNil(t, results[0])
	require.NoError(t, results[0].ExecErr)
	require.NotNil(t, results[0].SubLoop)
	require.True(t, results[0].SubLoop.DisablePeriodicVerification,
		"ExtraLoopOpts must be applied to the built loop before execution")
}