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

func TestFinishFirstRequestAlwaysContinuesAndQueuesCheckpoint(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	op := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, op)
	require.True(t, op.IsContinued())
	require.Equal(t, softTodoCheckpointPrompt, loop.consumeSoftTodoCheckpoint())
	require.Empty(t, loop.consumeSoftTodoCheckpoint())
}

func TestFinishSecondRequestExitsWithoutOpenTodos(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	first := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, first)
	_ = loop.consumeSoftTodoCheckpoint()
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
	_ = loop.consumeSoftTodoCheckpoint()
	second := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, second)
	require.True(t, second.IsContinued())
	require.Contains(t, second.GetFeedback().String(), "todo_delta.close")
	require.Empty(t, loop.consumeSoftTodoCheckpoint())
}

func TestNonFinishResetsFinishFlow(t *testing.T) {
	loop, _, _, task := newTodoGateTestLoop(t, nil)
	first := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, first)
	_ = loop.consumeSoftTodoCheckpoint()
	loop.resetSoftTodoFinishFlow()
	again := NewActionHandlerOperator(task)
	loopAction_Finish.ActionHandler(loop, nil, again)
	require.True(t, again.IsContinued())
	require.Equal(t, softTodoCheckpointPrompt, loop.consumeSoftTodoCheckpoint())
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
	require.Empty(t, loop.consumeSoftTodoCheckpoint())
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
