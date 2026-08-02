package reactloops

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils"
)

type softCheckpointConfig struct {
	*mock.MockedAIConfig
	mu                sync.Mutex
	active            []aicommon.VerificationTodoItem
	enableGoalMode    bool
	goalMinIterations int64
}

func (c *softCheckpointConfig) ActiveVerificationTodoItemsByScope(aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.active...)
}
func (c *softCheckpointConfig) GetEnableGoalMode() bool     { return c.enableGoalMode }
func (c *softCheckpointConfig) GetGoalMinIterations() int64 { return c.goalMinIterations }

type softCheckpointInvoker struct {
	*mock.MockInvoker
	cfg      *softCheckpointConfig
	task     aicommon.AIStatefulTask
	timeline []string
	results  []string
}

func (i *softCheckpointInvoker) GetConfig() aicommon.AICallerConfigIf        { return i.cfg }
func (i *softCheckpointInvoker) SetCurrentTask(task aicommon.AIStatefulTask) { i.task = task }
func (i *softCheckpointInvoker) GetCurrentTask() aicommon.AIStatefulTask     { return i.task }
func (i *softCheckpointInvoker) GetCurrentTaskId() string {
	if i.task == nil {
		return ""
	}
	return i.task.GetId()
}
func (i *softCheckpointInvoker) EmitResultAfterStream(result any) {
	i.results = append(i.results, strings.TrimSpace(utils.InterfaceToString(result)))
}
func (i *softCheckpointInvoker) AddToTimeline(category, line string) {
	i.timeline = append(i.timeline, category+": "+line)
}

func newTodoGateTestLoop(t *testing.T, active []aicommon.VerificationTodoItem) (*ReActLoop, *softCheckpointInvoker, *softCheckpointConfig, aicommon.AIStatefulTask) {
	t.Helper()
	base := mock.NewMockInvoker(context.Background())
	mockCfg, ok := base.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)
	cfg := &softCheckpointConfig{MockedAIConfig: mockCfg, active: active}
	invoker := &softCheckpointInvoker{MockInvoker: base, cfg: cfg}
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("task", "input", context.Background(), cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, invoker, cfg, task
}

func setCurrentTodo(t *testing.T, cfg *softCheckpointConfig, task aicommon.AIStatefulTask, id string) {
	t.Helper()
	current := id
	delta := &aicommon.TodoDelta{
		Current:    &current,
		CurrentSet: true,
		Add:        []aicommon.TodoAdd{{ID: id, Text: "work " + id}},
	}
	results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), delta)
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
}

func switchCurrentTodo(t *testing.T, cfg *softCheckpointConfig, task aicommon.AIStatefulTask, id string) {
	t.Helper()
	current := id
	delta := &aicommon.TodoDelta{
		Current:    &current,
		CurrentSet: true,
		Add:        []aicommon.TodoAdd{{ID: id, Text: "work " + id}},
	}
	results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), delta)
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
}

func TestFinishFirstRequestAlwaysContinuesAndQueuesCheckpoint(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, op)
	require.True(t, op.IsContinued())
	checkpoint := loop.consumeTodoCheckpoint()
	require.Equal(t, softTodoCheckpointPrompt, checkpoint)
	require.Contains(t, checkpoint, "单次阴性请求")
	require.Contains(t, checkpoint, "设为 CURRENT")
	require.Contains(t, checkpoint, "timeline 和最新 Observation")
	require.Contains(t, checkpoint, "TODO 已清空不是任务完成的充分证据")
	require.Contains(t, checkpoint, "仍属于当前用户目标和 CURRENT-TASK")
	require.Contains(t, checkpoint, "无需用户新增目标或授权")
	require.Contains(t, checkpoint, "预期信息增益很低")
	require.Contains(t, checkpoint, "立即用本轮 todo_delta")
	require.Contains(t, checkpoint, "尚未通过 todo_delta 进入 Frontier")
	require.Contains(t, checkpoint, "先将它们全部加入或更新到 Frontier")
	require.Contains(t, checkpoint, "具体目标、触发证据、可证伪假设和恢复后的第一步")
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestFinishSecondRequestExitsWithoutOpenTodos(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	first := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, first)
	_ = loop.consumeTodoCheckpoint()
	second := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, second)
	terminated, err := second.IsTerminated()
	require.True(t, terminated)
	require.NoError(t, err)
}

func TestFinishSecondRequestRejectsOpenTodosWithoutRepeatingCheckpoint(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{{ID: "todo-1", Content: "work", Status: aicommon.VerificationTodoStatusDoing}})
	first := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, first)
	_ = loop.consumeTodoCheckpoint()
	second := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, second)
	require.True(t, second.IsContinued())
	require.Contains(t, second.GetFeedback().String(), "todo_delta.close")
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestNonFinishResetsFinishFlow(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	first := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, first)
	_ = loop.consumeTodoCheckpoint()
	loop.resetSoftTodoFinishFlow()
	again := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, again)
	require.True(t, again.IsContinued())
	require.Equal(t, softTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
}

func TestGoalModeGatePrecedesCheckpoint(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	cfg.enableGoalMode = true
	cfg.goalMinIterations = 5
	loop.currentIterationIndex = 2
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, op)
	require.True(t, op.IsContinued())
	require.True(t, strings.Contains(op.GetFeedback().String(), "goal mode"))
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestDirectlyAnswerEmitsAndContinuesWithoutImplicitFinish(t *testing.T) {
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
	require.Equal(t, []string{"final"}, invoker.results)
}

func TestDirectlyAnswerWithOpenTodosStillEmitsAndContinues(t *testing.T) {
	loop, invoker, _, task := newTodoGateTestLoop(t, []aicommon.VerificationTodoItem{{ID: "todo-1", Content: "work", Status: aicommon.VerificationTodoStatusDoing}})
	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","answer_payload":"progress"}`, "directly_answer")
	require.NoError(t, err)
	require.NoError(t, loopAction_DirectlyAnswer.ActionVerifier(loop, action))

	op := NewActionHandlerOperator(task)
	loopAction_DirectlyAnswer.ActionHandler(loop, action, op)
	require.True(t, op.IsContinued())
	require.Equal(t, []string{"progress"}, invoker.results)
	require.Contains(t, op.GetFeedback().String(), "Remaining TODOs")
}

func TestCurrentTodoCheckpointQueuesAfterTwentyFifthValidIteration(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")

	for iteration := 0; iteration < currentTodoCheckpointThreshold-1; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}
	require.Empty(t, loop.consumeTodoCheckpoint(), "the 24th iteration must not queue a checkpoint")

	loop.recordCurrentTodoIteration(task)
	checkpoint := loop.consumeTodoCheckpoint()
	require.Equal(t, currentTodoCheckpointPrompt, checkpoint)
	require.Contains(t, checkpoint, "主要矛盾")
	require.Contains(t, checkpoint, "尚未进入 Frontier 的同级有效分支")
	require.Contains(t, checkpoint, "沿 CURRENT 继续向深处执行")
	require.Contains(t, checkpoint, "同一 todo_delta 中 close 旧项并设置下一 CURRENT")
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestCurrentTodoCheckpointRestartsWindowAfterInjection(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")

	for iteration := 0; iteration < currentTodoCheckpointThreshold; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}
	require.Equal(t, currentTodoCheckpointPrompt, loop.consumeTodoCheckpoint())

	for iteration := 0; iteration < currentTodoCheckpointThreshold-1; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}
	require.Empty(t, loop.consumeTodoCheckpoint())
	loop.recordCurrentTodoIteration(task)
	require.Equal(t, currentTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
}

func TestCurrentTodoCheckpointDropsWhenCurrentChangesBeforeInjection(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")
	for iteration := 0; iteration < currentTodoCheckpointThreshold; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}

	switchCurrentTodo(t, cfg, task, "todo-2")
	require.Empty(t, loop.consumeTodoCheckpoint())
	loop.recordCurrentTodoIteration(task)
	progress := loop.currentTodoProgress[todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(task))]
	require.NotNil(t, progress)
	require.Equal(t, "todo-2", progress.CurrentTodoID)
	require.Equal(t, 1, progress.Iterations)
}

func TestCurrentTodoCheckpointIsIsolatedByTaskScope(t *testing.T) {
	loop, invoker, cfg, firstTask := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, firstTask, "todo-1")
	for iteration := 0; iteration < currentTodoCheckpointThreshold; iteration++ {
		loop.recordCurrentTodoIteration(firstTask)
	}

	secondTask := aicommon.NewStatefulTaskBase("task-2", "input", context.Background(), cfg.GetEmitter(), true)
	setCurrentTodo(t, cfg, secondTask, "todo-2")
	invoker.SetCurrentTask(secondTask)
	loop.SetCurrentTask(secondTask)
	require.Empty(t, loop.consumeTodoCheckpoint())

	invoker.SetCurrentTask(firstTask)
	loop.SetCurrentTask(firstTask)
	require.Equal(t, currentTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
}

func TestFinishCheckpointSubsumesPendingCurrentCheckpoint(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")
	for iteration := 0; iteration < currentTodoCheckpointThreshold; iteration++ {
		loop.recordCurrentTodoIteration(task)
	}

	require.False(t, loop.requestSoftTodoCheckpoint())
	require.Equal(t, softTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
	require.Empty(t, loop.consumeTodoCheckpoint())
}
