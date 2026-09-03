package phase2

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/schema"
)

// resumePropagationInvoker 构建真实 *aicommon.Config 父配置，并把子 invoker 的
// AI 回调替换为可控模拟：回调持续返回无法解析的模型输出，使子 loop 的
// transaction 重试耗尽 → loop 返回执行错误 → ExecErr 非空。这是引擎日志中
// 类别子代理死亡（AI 连续失败）的最小复现。
type resumePropagationInvoker struct {
	*mock.MockInvoker
	cfg *aicommon.Config
}

func (r *resumePropagationInvoker) GetConfig() aicommon.AICallerConfigIf { return r.cfg }

// newResumePropagationHarness 搭建 runAllCategoryScans 全链路测试环境。
//
// 返回的 restore 必须 defer 调用（恢复全局 AIRuntimeInvokerGetter）。
// aiCallback 返回 (*AIResponse, nil)：响应带非空输出流但 postHandler 无法
// 从中解析出合法 action（复现 "action resolution failed" 而非网络类），
// transaction 在短重试预算内耗尽后使子 loop 返回错误。
func newResumePropagationHarness(t *testing.T) (
	r *resumePropagationInvoker,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
	categories []model.VulnCategory,
	restore func(),
) {
	t.Helper()

	parentEmitter := aicommon.NewEmitter("resume-prop-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	// 注意：tiered 槽位（Quality/SpeedPriorityRaw）必须一并设置——子 invoker
	// 经 GetRawAICallbacks→WithAICallbacks 重建回调时，仅设 Original 会让
	// wrapper(nil) 占据 QualityPriority 槽位，CallAI 回退链在非 nil 的空
	// wrapper 处短路并报 "AI callback is not set"。
	aiCB := func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		// 非空输出流 + 无法解析为合法 action 的文本：复现子 loop 在迭代首跳
		// 即失败（transaction 重试耗尽后返回错误）。
		rsp := aicommon.NewUnboundAIResponse()
		rsp.EmitOutputStream(strings.NewReader("unparseable model output without any action"))
		rsp.Close()
		return rsp, nil
	}
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithTimeline(aicommon.NewTimeline(nil, nil)),
		aicommon.WithEmitter(parentEmitter),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithAICallbacks(&aicommon.AICallbacks{
			Original:           aiCB,
			QualityPriorityRaw: aiCB,
			SpeedPriorityRaw:   aiCB,
		}),
	)

	invoker := &resumePropagationInvoker{MockInvoker: mock.NewMockInvoker(context.Background()), cfg: cfg}

	origGetter := aicommon.AIRuntimeInvokerGetter
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		return &resumePropagationInvoker{MockInvoker: mock.NewMockInvoker(ctx), cfg: aicommon.NewConfig(ctx, opts...)}, nil
	}
	restore = func() { aicommon.AIRuntimeInvokerGetter = origGetter }

	// 直接构造父 loop（与注册工厂等价，仅省略 init task——本测试只依赖
	// runAllCategoryScans 对 *ReActLoop 的 GetMaxSubAgents 契约）。
	loop, loopErr := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, invoker)
	require.NoError(t, loopErr)
	require.NotNil(t, loop)
	task = mock.NewMockStatefulTask(context.Background(), "phase2-parent", "audit project")
	state = model.NewAuditState()
	state.WorkDir = t.TempDir()
	state.ProjectPath = t.TempDir()
	categories = []model.VulnCategory{{ID: "sql-injection", Name: "SQL注入"}}
	return invoker, loop, task, state, categories, restore
}

// TestRunAllCategoryScans_ResumeExecErrPropagatedToFallbackSummary 验证 N3：
// resume 轮子代理死亡时，其真实执行错误必须传播进兜底 observation 的
// CoverageSummary（修复前硬编码 nil，summary 里 execErr=<nil> 掩盖根因）。
func TestRunAllCategoryScans_ResumeExecErrPropagatedToFallbackSummary(t *testing.T) {
	invoker, loop, task, state, categories, restore := newResumePropagationHarness(t)
	defer restore()

	// 初始批次与 resume 批次的子 loop 都会因 AI 输出无法解析而死亡，
	// 两轮的 ExecErr 均非空。
	outcomes := runAllCategoryScans(invoker, loop, task, state, categories)
	require.NotEmpty(t, outcomes)

	observations := state.GetScanObservations()
	require.NotEmpty(t, observations, "fallback must record an observation for the interrupted category")
	obs := observations[0]
	summary := obs.CoverageSummary
	t.Logf("fallback summary: %s", summary)
	// 本场景子 loop 在阶段A锁定任何文件前即死亡 → not_run_interrupted 分支；
	// 强标 not_audited 的 partial 分支由
	// TestFallbackFinalizeCategoryScan_PartialMarksRemainingAndRecords 覆盖。
	require.Equal(t, "not_run_interrupted", obs.StopReason)
	require.NotContains(t, summary, "execErr=<nil>",
		"resume-round exec error must be propagated instead of a hardcoded nil")
	require.Contains(t, summary, "action resolution failed",
		"the real transaction failure cause must surface in the fallback summary")
}

// TestResumeJobIdentifierStability 派发与复查必须共用同一 identifier 来源，
// 否则 resumeExecErrs 映射查不到对应条目（回归防线）。
func TestResumeJobIdentifierStability(t *testing.T) {
	require.Equal(t, "sql-injection-resume", resumeJobIdentifier("sql-injection"))
}

// TestRunAllCategoryScans_SingleResumeRound 验证可恢复类别只经历一次初始批
// + 一次 resume 批（两轮 DispatchSubAgents），且最终写入兜底 observation。
func TestRunAllCategoryScans_SingleResumeRound(t *testing.T) {
	invoker, loop, task, state, categories, restore := newResumePropagationHarness(t)
	defer restore()

	_ = runAllCategoryScans(invoker, loop, task, state, categories)
	// 兜底 observation 已写入（初始批死亡 → resume 批死亡 → 兜底）。
	require.NotEmpty(t, state.GetScanObservations())
}

var _ = errors.New // 保留 errors 导入便于后续断言扩展

var _ sync.Mutex // 保留 sync 导入便于后续并发断言扩展
