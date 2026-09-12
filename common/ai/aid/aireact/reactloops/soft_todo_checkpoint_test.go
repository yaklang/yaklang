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

func (c *softCheckpointConfig) ActiveVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		return append([]aicommon.VerificationTodoItem(nil), c.active...)
	}
	return c.MockedAIConfig.ActiveVerificationTodoItemsByScope(scope)
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

func TestFinishWithoutOpenTodosRequiresCompletionReview(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestFinishWithOpenTodosBlocksUntilTheyAreClosed(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	setCurrentTodo(t, cfg, task, "todo-1")
	// Repeating finish never bypasses remaining work or opens a count gate.
	for attempt := 0; attempt < 3; attempt++ {
		op := NewActionHandlerOperator(task)
		loopAction_Finish.ActionHandler(loop, nil, op)
		require.True(t, op.IsContinued())
		require.Contains(t, op.GetFeedback().String(), "Remaining TODOs")
		require.Contains(t, op.GetFeedback().String(), "todo-1")
		require.Contains(t, op.GetFeedback().String(), "Do not retry finish")
		require.Equal(t, finishTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
		require.Empty(t, loop.consumeTodoCheckpoint())
	}

	results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{
		Close: []aicommon.TodoClose{{ID: "todo-1", Outcome: aicommon.TodoOutcomeResolved, Reason: "targeted check passed", Refs: []string{"observation-1"}}},
	})
	require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
	requireCompletionCheckpoint(t, loop, task)
	requireReviewedFinish(t, loop, task)
	require.Empty(t, loop.consumeTodoCheckpoint())
}

func TestFinishCheckpointOnlyAppliesToRemainingTodosInRequestingScope(t *testing.T) {
	for _, change := range []string{"resolved", "different task"} {
		t.Run(change, func(t *testing.T) {
			loop, _, cfg, task := newTodoGateTestLoop(t, nil)
			setCurrentTodo(t, cfg, task, "todo-1")
			op := NewActionHandlerOperator(task)
			loopAction_Finish.ActionHandler(loop, nil, op)
			require.True(t, op.IsContinued())
			if change == "resolved" {
				results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{
					Close: []aicommon.TodoClose{{ID: "todo-1", Outcome: aicommon.TodoOutcomeResolved, Reason: "completed"}},
				})
				require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
			} else {
				other := aicommon.NewStatefulTaskBase("other-task", "input", context.Background(), cfg.GetEmitter(), true)
				setCurrentTodo(t, cfg, other, "other-todo")
				loop.SetCurrentTask(other)
			}
			require.Empty(t, loop.consumeTodoCheckpoint(), "a stale finish request must not trigger a checkpoint")
		})
	}
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
	require.Contains(t, op.GetFeedback().String(), "host-side completion gate")
	require.NotContains(t, op.GetFeedback().String(), "iteration")
	require.NotContains(t, op.GetFeedback().String(), "5")
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
	require.Empty(t, op.GetFeedback().String(), "a progress answer must not trigger the finish gate")
	require.Empty(t, loop.consumeTodoCheckpoint())
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
	require.Contains(t, checkpoint, "保持开放并切换 current")
	require.NotContains(t, checkpoint, "25")
	require.NotContains(t, strings.ToLower(checkpoint), "iteration")
	require.NotContains(t, checkpoint, "迭代")
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

	loopAction_Finish.ActionHandler(loop, nil, NewActionHandlerOperator(task))
	require.Equal(t, finishTodoCheckpointPrompt, loop.consumeTodoCheckpoint())
	require.Empty(t, loop.consumeTodoCheckpoint())
}
