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

// maxIterTestConfig 使用真实的 canonical TODO store，验证到达迭代上限时
// 活跃 TODO 是否以带 reason 的 deferred 结果关闭。
type maxIterTestConfig struct {
	*mock.MockedAIConfig
}

type maxIterTestInvoker struct {
	*mock.MockInvoker
	cfg         *maxIterTestConfig
	currentTask aicommon.AIStatefulTask

	mu       sync.Mutex
	timeline []string
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

func newMaxIterTestLoop(t *testing.T, active []aicommon.VerificationTodoItem) (*ReActLoop, *maxIterTestInvoker, *maxIterTestConfig, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	baseInvoker := mock.NewMockInvoker(ctx)
	mockCfg, ok := baseInvoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)

	cfg := &maxIterTestConfig{MockedAIConfig: mockCfg}
	invoker := &maxIterTestInvoker{
		MockInvoker: baseInvoker,
		cfg:         cfg,
	}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("test-task", "分析这批 HTTP 流量里有没有敏感信息泄露", ctx, cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	if len(active) > 0 {
		adds := make([]aicommon.TodoAdd, 0, len(active))
		for _, item := range active {
			adds = append(adds, aicommon.TodoAdd{ID: item.ID, Text: item.Content})
		}
		results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{Add: adds})
		for _, result := range results {
			require.True(t, result.Success, result.Reason)
		}
	}
	return loop, invoker, cfg, task
}

// TestMaxIterationSoftInterrupt_DefersActiveTodos 验证到达迭代上限的软性中断:
// 当前任务仍开放的 TODO 会被批量关闭为 deferred，并留下明确 reason。
func TestMaxIterationSoftInterrupt_DefersActiveTodos(t *testing.T) {
	loop, invoker, cfg, task := newMaxIterTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "check_sensitive", Content: "检查响应体里是否有身份证/手机号", Status: aicommon.VerificationTodoStatusDoing},
		{ID: "check_token", Content: "确认是否有明文 token 泄露", Status: aicommon.VerificationTodoStatusPending},
	})

	loop.applyMaxIterationSoftInterrupt(11, task, 10)

	open, current, closed := cfg.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Empty(t, open)
	require.Empty(t, current)
	require.Len(t, closed, 2)
	closedIDs := []string{closed[0].ID, closed[1].ID}
	require.ElementsMatch(t, []string{"check_sensitive", "check_token"}, closedIDs)
	for _, item := range closed {
		require.Equal(t, aicommon.TodoOutcomeDeferred, item.Outcome)
		require.NotEmpty(t, strings.TrimSpace(item.Reason))
	}

	// 软中断标记与未完成快照
	require.True(t, loop.IsMaxIterationInterrupted())
	summary := loop.GetMaxIterationInterruptSummary()
	assert.Contains(t, summary, "检查响应体里是否有身份证/手机号")
	assert.Contains(t, summary, "确认是否有明文 token 泄露")

	// 单条软性中断 timeline 说明
	assert.Contains(t, invoker.timelineString(), "execution_paused")
	assert.NotContains(t, invoker.timelineString(), "iteration limit")
}

// TestMaxIterationSoftInterrupt_NoActiveTodos 验证没有活跃 TODO 时也能安全走软性
// 中断: 不产生关闭项, 但仍置位中断标记并落一条软性说明.
func TestMaxIterationSoftInterrupt_NoActiveTodos(t *testing.T) {
	loop, invoker, cfg, task := newMaxIterTestLoop(t, nil)

	loop.applyMaxIterationSoftInterrupt(11, task, 10)

	_, _, closed := cfg.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	require.Empty(t, closed)
	require.True(t, loop.IsMaxIterationInterrupted())
	assert.Empty(t, loop.GetMaxIterationInterruptSummary())
	assert.Contains(t, invoker.timelineString(), "execution_paused")
	assert.NotContains(t, invoker.timelineString(), "iteration limit")
}

// TestClassifyLoopFinishEmission_SoftInterruptIsNaturalEnd 覆盖框架层全局收尾的核心
// 决策 (测试要点 1/2/4):
//   - 到达迭代上限的软性中断 -> "自然结束"(success), 不报错 (中断原因/未完成 TODO/
//     下一步建议由各 loop 已有的 finalize 收尾总结承载, 框架不额外发 AI 请求);
//   - 对比硬中断 (已结束且带错误、非软中断) -> 硬失败 fail (携带错误信息);
//   - IgnoreError (隐藏/内部 loop 自管收尾) -> 静默.
//
// 关键词: max iteration 软中断 自然结束, 不报错, 对比硬中断报错
func TestClassifyLoopFinishEmission_SoftInterruptIsNaturalEnd(t *testing.T) {
	maxIterErr := utils.Errorf("reached max iterations (10), stopping test loop")

	// 要点1&2: 软性中断 -> 自然结束(success), 即便 reason 带着 maxIterErr 也不报 fail
	got := ClassifyLoopFinishEmission(true, maxIterErr, false, true)
	require.Equal(t, LoopFinishSuccess, got,
		"max-iteration soft interrupt must be a natural success, not a failure")
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
