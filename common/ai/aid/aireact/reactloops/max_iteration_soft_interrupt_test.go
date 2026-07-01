package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils"
)

// maxIterTestConfig 记录 ApplyVerificationTodoOps 调用, 并按 scope 返回活跃 TODO,
// 用来验证"到达迭代上限"时的软性中断是否把活跃 TODO 批量标记为 SKIP.
type maxIterTestConfig struct {
	*mock.MockedAIConfig

	mu           sync.Mutex
	activeByTask map[string][]aicommon.VerificationTodoItem
	appliedOps   []aicommon.VerifyNextMovement
	appliedScope aicommon.VerificationTodoScope
}

func (c *maxIterTestConfig) ActiveVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.activeByTask[scope.TaskID]...)
}

func (c *maxIterTestConfig) ApplyVerificationTodoOps(scope aicommon.VerificationTodoScope, satisfied bool, movements []aicommon.VerifyNextMovement) []aicommon.VerificationTodoApplyError {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.appliedScope = scope
	c.appliedOps = append(c.appliedOps, movements...)
	return nil
}

func (c *maxIterTestConfig) appliedSkipIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.appliedOps))
	for _, m := range c.appliedOps {
		if strings.EqualFold(strings.TrimSpace(m.Op), "skip") {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

type maxIterTestInvoker struct {
	*mock.MockInvoker
	cfg         *maxIterTestConfig
	currentTask aicommon.AIStatefulTask

	mu          sync.Mutex
	timeline    []string
	emitResults []string
}

func (i *maxIterTestInvoker) GetConfig() aicommon.AICallerConfigIf { return i.cfg }

func (i *maxIterTestInvoker) SetCurrentTask(task aicommon.AIStatefulTask) { i.currentTask = task }

func (i *maxIterTestInvoker) GetCurrentTask() aicommon.AIStatefulTask { return i.currentTask }

func (i *maxIterTestInvoker) GetCurrentTaskId() string {
	if i.currentTask == nil {
		return ""
	}
	return i.currentTask.GetId()
}

func (i *maxIterTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timeline = append(i.timeline, entry+": "+content)
}

func (i *maxIterTestInvoker) timelineString() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return strings.Join(i.timeline, "\n")
}

func (i *maxIterTestInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.emitResults = append(i.emitResults, utils.InterfaceToString(result))
}

func (i *maxIterTestInvoker) emitResultString() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return strings.Join(i.emitResults, "\n")
}

func newMaxIterTestLoop(t *testing.T, active []aicommon.VerificationTodoItem) (*ReActLoop, *maxIterTestInvoker, *maxIterTestConfig, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	baseInvoker := mock.NewMockInvoker(ctx)
	mockCfg, ok := baseInvoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)

	cfg := &maxIterTestConfig{
		MockedAIConfig: mockCfg,
		activeByTask: map[string][]aicommon.VerificationTodoItem{
			"test-task": active,
		},
	}
	invoker := &maxIterTestInvoker{
		MockInvoker: baseInvoker,
		cfg:         cfg,
	}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("test-task", "分析这批 HTTP 流量里有没有敏感信息泄露", ctx, cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, invoker, cfg, task
}

// TestMaxIterationSoftInterrupt_MarksActiveTodosSkip 验证到达迭代上限的软性中断:
// 当前任务仍活跃的 TODO 会被批量标记为 SKIP, 未完成快照被记录, 且 Timeline 落一
// 条软性中断说明 (非 error), 供直接回答引用. 关键词: max iteration 软中断, 待办 SKIP
func TestMaxIterationSoftInterrupt_MarksActiveTodosSkip(t *testing.T) {
	loop, invoker, cfg, task := newMaxIterTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "check_sensitive", Content: "检查响应体里是否有身份证/手机号", Status: aicommon.VerificationTodoStatusDoing},
		{ID: "check_token", Content: "确认是否有明文 token 泄露", Status: aicommon.VerificationTodoStatusPending},
	})

	loop.applyMaxIterationSoftInterrupt(11, task, 10)

	// 两条活跃 TODO 都应被标记为 SKIP
	skipIDs := cfg.appliedSkipIDs()
	require.ElementsMatch(t, []string{"check_sensitive", "check_token"}, skipIDs)
	require.Equal(t, "test-task", cfg.appliedScope.TaskID)

	// 软中断标记与未完成快照
	require.True(t, loop.IsMaxIterationInterrupted())
	summary := loop.GetMaxIterationInterruptSummary()
	assert.Contains(t, summary, "检查响应体里是否有身份证/手机号")
	assert.Contains(t, summary, "确认是否有明文 token 泄露")

	// 单条软性中断 timeline 说明
	assert.Contains(t, invoker.timelineString(), "iteration_limit_interrupt")
}

// TestMaxIterationSoftInterrupt_NoActiveTodos 验证没有活跃 TODO 时也能安全走软性
// 中断: 不产生 skip op, 但仍置位中断标记并落一条软性说明.
func TestMaxIterationSoftInterrupt_NoActiveTodos(t *testing.T) {
	loop, invoker, cfg, task := newMaxIterTestLoop(t, nil)

	loop.applyMaxIterationSoftInterrupt(11, task, 10)

	require.Empty(t, cfg.appliedSkipIDs())
	require.True(t, loop.IsMaxIterationInterrupted())
	assert.Empty(t, loop.GetMaxIterationInterruptSummary())
	assert.Contains(t, invoker.timelineString(), "iteration_limit_interrupt")
}

// TestClassifyLoopFinishEmission_SoftInterruptIsNaturalEnd 覆盖框架层全局收尾的核心
// 决策 (测试要点 1/2/4):
//   - 到达迭代上限的软性中断 -> "自然结束"(success) + 补发中断说明, 不报错;
//   - 对比硬中断 (已结束且带错误、非软中断) -> 硬失败 fail (携带错误信息);
//   - IgnoreError (隐藏/内部 loop 自管收尾) -> 静默.
//
// 关键词: max iteration 软中断 自然结束, 不报错, 对比硬中断报错
func TestClassifyLoopFinishEmission_SoftInterruptIsNaturalEnd(t *testing.T) {
	maxIterErr := utils.Errorf("reached max iterations (10), stopping test loop")

	// 要点1: 软性中断 -> 自然结束(success) + 补发中断说明, 不是 fail
	got := ClassifyLoopFinishEmission(true, maxIterErr, false, true)
	require.Equal(t, LoopFinishSuccessWithInterruptSummary, got,
		"max-iteration soft interrupt must be a natural success with an interrupt summary, not a failure")

	// 要点2: 软性中断不产生 fail
	require.NotEqual(t, LoopFinishFail, got, "soft interrupt must not be reported as failure")

	// 要点4: 对比硬中断 (真实错误, 未标记软中断) -> 携带错误信息的 fail
	hard := ClassifyLoopFinishEmission(true, utils.Errorf("boom: unexpected transaction error"), false, false)
	require.Equal(t, LoopFinishFail, hard, "a genuine hard interrupt/error must still be reported as failure")

	// 隐藏/内部 loop 自管收尾 (IgnoreError) -> 静默, 即便标记了软中断也不打扰
	silent := ClassifyLoopFinishEmission(true, maxIterErr, true, true)
	require.Equal(t, LoopFinishSilent, silent, "IgnoreError loops must stay silent")

	// 正常完成 (无错误) -> 成功
	ok := ClassifyLoopFinishEmission(true, nil, false, false)
	require.Equal(t, LoopFinishSuccess, ok)
}

// TestDeliverMaxIterationInterruptSummary_FallbackExplainsAndAsksNext 验证框架层补发
// 的中断说明 (测试要点 1): 即便 AI 不可用走兜底, 也会明确"因迭代上限自然结束(非错误)"、
// 列出未完成 TODO、并提示用户可回复 "继续" 或开启新话题; 同时落一条 timeline.
// 关键词: 框架中断说明, 自然结束, 未完成 TODO, 询问下一步(继续)
func TestDeliverMaxIterationInterruptSummary_FallbackExplainsAndAsksNext(t *testing.T) {
	loop, invoker, _, task := newMaxIterTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "check_sensitive", Content: "检查响应体里是否有身份证/手机号", Status: aicommon.VerificationTodoStatusDoing},
	})

	// 先触发软性中断以记录"未完成 TODO"快照
	loop.applyMaxIterationSoftInterrupt(21, task, 20)

	// mock 的 InvokeSpeedPriorityLiteForge 不会返回可用的 continuation_note,
	// 因此会走极简兜底文案 —— 兜底同样承担"解释中断 + 询问下一步"的职责.
	loop.DeliverMaxIterationInterruptSummary()

	result := invoker.emitResultString()
	assert.Contains(t, result, "迭代次数上限")
	assert.Contains(t, result, "继续")
	assert.Contains(t, result, "检查响应体里是否有身份证/手机号")

	assert.Contains(t, invoker.timelineString(), "iteration_limit_interrupt_summary")
}
