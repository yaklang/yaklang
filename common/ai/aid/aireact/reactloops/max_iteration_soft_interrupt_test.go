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

// TestFinishIterationLoopWithSoftInterrupt_SuppressesFail 验证软性中断收尾会预置
// IgnoreError: 即便某个 loop 没有 finalize hook 主动 IgnoreError, ExecuteLoopTask
// 里"reason != nil 且未 IgnoreError -> EmitReActFail"的全局兜底也一定观察到 ignore,
// 从而不会把"到达迭代上限"当成任务失败上报.
// 关键词: 软性中断预置 IgnoreError, 抑制 EmitReActFail
func TestFinishIterationLoopWithSoftInterrupt_SuppressesFail(t *testing.T) {
	loop, _, _, task := newMaxIterTestLoop(t, nil)

	var wouldFail bool
	// 模拟 ExecuteLoopTask 里注册的全局 fail 兜底: 用 DeferAfterCallbacks 在所有
	// 回调跑完后再判定, 与线上时序一致.
	WithOnPostIteraction(func(_ *ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
		operator.DeferAfterCallbacks(func() {
			if isDone && reason != nil && !operator.ShouldIgnoreError() {
				wouldFail = true
			}
		})
	})(loop)

	loop.finishIterationLoopWithSoftInterrupt(11, task, utils.Errorf("reached max iterations (10), stopping test loop"))

	require.False(t, wouldFail, "soft interrupt must not trigger EmitReActFail")
}

// TestFinishIterationLoopWithError_WouldFailWithoutIgnore 作为对照: 老的
// finishIterationLoopWithError 在没有任何 hook 主动 IgnoreError 时, 全局兜底会判定
// 为失败. 这正是软性中断路径要修掉的坏体验.
func TestFinishIterationLoopWithError_WouldFailWithoutIgnore(t *testing.T) {
	loop, _, _, task := newMaxIterTestLoop(t, nil)

	var wouldFail bool
	WithOnPostIteraction(func(_ *ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
		operator.DeferAfterCallbacks(func() {
			if isDone && reason != nil && !operator.ShouldIgnoreError() {
				wouldFail = true
			}
		})
	})(loop)

	loop.finishIterationLoopWithError(11, task, utils.Errorf("reached max iterations (10), stopping test loop"))

	require.True(t, wouldFail, "legacy error path would emit fail without an IgnoreError hook")
}
