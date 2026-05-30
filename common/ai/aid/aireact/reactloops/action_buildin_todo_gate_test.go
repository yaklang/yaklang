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

type todoGateTestConfig struct {
	*mock.MockedAIConfig

	mu           sync.Mutex
	activeByTask map[string][]aicommon.VerificationTodoItem
}

func (c *todoGateTestConfig) ActiveVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.activeByTask[scope.TaskID]...)
}

type todoGateTestInvoker struct {
	*mock.MockInvoker
	cfg         *todoGateTestConfig
	currentTask aicommon.AIStatefulTask

	mu       sync.Mutex
	results  []string
	timeline []string
}

func (i *todoGateTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.cfg
}

func (i *todoGateTestInvoker) SetCurrentTask(task aicommon.AIStatefulTask) {
	i.currentTask = task
}

func (i *todoGateTestInvoker) GetCurrentTask() aicommon.AIStatefulTask {
	return i.currentTask
}

func (i *todoGateTestInvoker) GetCurrentTaskId() string {
	if i.currentTask == nil {
		return ""
	}
	return i.currentTask.GetId()
}

func (i *todoGateTestInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.results = append(i.results, strings.TrimSpace(utils.InterfaceToString(result)))
}

func (i *todoGateTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timeline = append(i.timeline, entry+": "+content)
}

func (i *todoGateTestInvoker) timelineString() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return strings.Join(i.timeline, "\n")
}

func newTodoGateTestLoop(t *testing.T, active []aicommon.VerificationTodoItem) (*ReActLoop, *todoGateTestInvoker, *todoGateTestConfig, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	baseInvoker := mock.NewMockInvoker(ctx)
	mockCfg, ok := baseInvoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)

	cfg := &todoGateTestConfig{
		MockedAIConfig: mockCfg,
		activeByTask: map[string][]aicommon.VerificationTodoItem{
			"test-task": active,
		},
	}
	invoker := &todoGateTestInvoker{
		MockInvoker: baseInvoker,
		cfg:         cfg,
	}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("test-task", "user input", ctx, cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, invoker, cfg, task
}

// TestDirectlyAnswer_EmitsAndContinuesWithOpenTodos 验证去 Exit 化后的核心语义:
// directly_answer 绝不再因为有未关闭 TODO 而拦截 emit. 即便当前任务仍有 open
// TODO 且本动作未携带 next_movements, 答复仍必须 emit, operator 必须 Continue
// (永不 Exit), 并通过 Feedback 提醒 AI 先关 TODO 再用 finish 收口.
// 关键词: directly_answer 永不 Exit, 仍 emit + Continue, finish 唯一终结器
func TestDirectlyAnswer_EmitsAndContinuesWithOpenTodos(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "collect_logs", Content: "收集日志", Status: aicommon.VerificationTodoStatusPending},
	})
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"final"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	// 答复必须 emit, 不再被 blocked-by-todo 闸门拦下
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "final", invoker.results[0])
	// open TODO 且无增量 -> Feedback 提醒先关 TODO 再 finish
	assert.Contains(t, op.GetFeedback().String(), "Remaining TODOs")
	// 不再产生隐式收口拦截标记
	assert.NotContains(t, invoker.timelineString(), "[DIRECT_ANSWER_BLOCKED_BY_TODO]")
	assert.Contains(t, invoker.timelineString(), "directly_answer")
}

func TestFinish_BlockedByCurrentTaskTodos(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "write_summary", Content: "补齐总结", Status: aicommon.VerificationTodoStatusDoing},
	})
	op := NewActionHandlerOperator(task)

	loopAction_Finish.ActionHandler(loop, nil, op)

	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	assert.Contains(t, op.GetFeedback().String(), "finish cannot exit")
	assert.Contains(t, invoker.timelineString(), "[FINISH_BLOCKED_BY_TODO]")
	assert.Contains(t, invoker.timelineString(), "write_summary")
}

// TestDirectlyAnswer_EmitsAndContinuesWhenTodosClosed 验证无 open TODO 时
// directly_answer 同样只 emit + Continue, 不再 Exit. 终结改由显式 finish 负责.
// 关键词: directly_answer 永不 Exit, 无 TODO 也续跑
func TestDirectlyAnswer_EmitsAndContinuesWhenTodosClosed(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, nil)
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"final"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "final", invoker.results[0])
	assert.Contains(t, invoker.timelineString(), "directly_answer")
}

// TestDirectlyAnswer_ContinuesWhenNextMovementsLeaveOpenTodos 验证携带
// next_movements 且仍有 open TODO 时: 答复 emit + Continue 续跑推进剩余 TODO,
// 永不 Exit.
// 关键词: directly_answer 永不 Exit, 携带增量续跑
func TestDirectlyAnswer_ContinuesWhenNextMovementsLeaveOpenTodos(t *testing.T) {
	// active items 代表 "apply 之后" 仍有一条未关闭 TODO
	loop, invoker, _, task := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{
		{ID: "deep_scan", Content: "继续深入扫描", Status: aicommon.VerificationTodoStatusDoing},
	})
	action, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"partial answer","next_movements":[{"op":"add","id":"deep_scan","content":"继续深入扫描"}]}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	// 携带增量且仍有 open -> 续跑, 且答复已 emit
	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "partial answer", invoker.results[0])
	assert.NotContains(t, invoker.timelineString(), "[DIRECT_ANSWER_BLOCKED_BY_TODO]")
}

// TestDirectlyAnswer_ContinuesEvenWhenNextMovementsCloseAllTodos 验证去 Exit 化:
// 即便 next_movements 把活全 done/delete/skip 关掉、apply 之后已无 open TODO,
// directly_answer 也只 emit + Continue, 绝不 Exit. 真正终结仅由显式 finish 完成.
// 关键词: directly_answer 永不 Exit, 增量关全仍续跑, finish 唯一终结器
func TestDirectlyAnswer_ContinuesEvenWhenNextMovementsCloseAllTodos(t *testing.T) {
	// active items 为空, 代表 "apply 之后" 已无未关闭 TODO
	loop, invoker, _, task := newTodoGateTestLoop(t, nil)
	action, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"all done","next_movements":[{"op":"done","id":"deep_scan"}]}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	// 即使增量关全, 也只续跑不收口; 答复已 emit
	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "all done", invoker.results[0])
}
