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

func TestDirectlyAnswer_BlockedByCurrentTaskTodos(t *testing.T) {
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
	assert.Empty(t, invoker.results)
	assert.Contains(t, op.GetFeedback().String(), "Remaining TODOs")
	assert.Contains(t, invoker.timelineString(), "[DIRECT_ANSWER_BLOCKED_BY_TODO]")
	assert.Contains(t, invoker.timelineString(), "collect_logs")
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

func TestDirectlyAnswer_ExitsWhenCurrentTaskTodosClosed(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, nil)
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"final"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)

	terminated, termErr := op.IsTerminated()
	require.True(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "final", invoker.results[0])
	assert.Contains(t, invoker.timelineString(), "directly_answer")
}
