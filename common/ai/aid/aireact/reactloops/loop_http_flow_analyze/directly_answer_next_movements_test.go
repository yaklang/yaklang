package loop_http_flow_analyze

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// flowAnalyzeTodoConfig 暴露可配置的 active TODO 集合, 用来模拟主循环
// applyNextMovementsBottomLine apply 之后的 store 状态.
type flowAnalyzeTodoConfig struct {
	*mock.MockedAIConfig
	mu     sync.Mutex
	active map[string][]aicommon.VerificationTodoItem
}

func (c *flowAnalyzeTodoConfig) ActiveVerificationTodoItemsByScope(scope aicommon.VerificationTodoScope) []aicommon.VerificationTodoItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aicommon.VerificationTodoItem(nil), c.active[scope.TaskID]...)
}

type flowAnalyzeTodoInvoker struct {
	*mock.MockInvoker
	cfg         *flowAnalyzeTodoConfig
	currentTask aicommon.AIStatefulTask

	mu      sync.Mutex
	results []string
}

func (i *flowAnalyzeTodoInvoker) GetConfig() aicommon.AICallerConfigIf { return i.cfg }

func (i *flowAnalyzeTodoInvoker) SetCurrentTask(task aicommon.AIStatefulTask) { i.currentTask = task }

func (i *flowAnalyzeTodoInvoker) GetCurrentTask() aicommon.AIStatefulTask { return i.currentTask }

func (i *flowAnalyzeTodoInvoker) GetCurrentTaskId() string {
	if i.currentTask == nil {
		return ""
	}
	return i.currentTask.GetId()
}

func (i *flowAnalyzeTodoInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.results = append(i.results, strings.TrimSpace(utils.InterfaceToString(result)))
}

func newFlowAnalyzeTodoLoop(t *testing.T, active []aicommon.VerificationTodoItem) (*reactloops.ReActLoop, *flowAnalyzeTodoInvoker, aicommon.AIStatefulTask) {
	t.Helper()
	ctx := context.Background()
	base := mock.NewMockInvoker(ctx)
	mockCfg, ok := base.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)

	cfg := &flowAnalyzeTodoConfig{
		MockedAIConfig: mockCfg,
		active:         map[string][]aicommon.VerificationTodoItem{"flow-task": active},
	}
	invoker := &flowAnalyzeTodoInvoker{MockInvoker: base, cfg: cfg}
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("flow-task", "分析这段流量", ctx, cfg.GetEmitter(), true)
	invoker.SetCurrentTask(task)
	loop.SetCurrentTask(task)
	return loop, invoker, task
}

// TestFlowAnalyzeDirectlyAnswer_ContinuesWithNextMovements 验证 http_flow_analyze
// 专用 directly_answer 复用了与 buildin 同一套收口决策: 携带 next_movements 且
// apply 之后仍有 open TODO 时, 答复后续跑而非收口.
func TestFlowAnalyzeDirectlyAnswer_ContinuesWithNextMovements(t *testing.T) {
	loop, invoker, task := newFlowAnalyzeTodoLoop(t, []aicommon.VerificationTodoItem{
		{ID: "trace_followups", Content: "继续追踪关联请求", Status: aicommon.VerificationTodoStatusDoing},
	})
	action, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"阶段性结论","next_movements":[{"op":"add","id":"trace_followups","content":"继续追踪关联请求"}]}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopActionDirectlyAnswerHTTPFlowAnalyze.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	loopActionDirectlyAnswerHTTPFlowAnalyze.ActionHandler(loop, action, op)

	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "阶段性结论", invoker.results[0])
}

// TestFlowAnalyzeDirectlyAnswer_ContinuesWithoutNextMovements 验证去 Exit 化:
// 不携带 next_movements 时, http_flow_analyze 专用 directly_answer 同样只
// emit + Continue, 绝不 Exit. 终结改由显式 finish 负责.
// 关键词: directly_answer 永不 Exit, http_flow_analyze 无增量也续跑
func TestFlowAnalyzeDirectlyAnswer_ContinuesWithoutNextMovements(t *testing.T) {
	loop, invoker, task := newFlowAnalyzeTodoLoop(t, nil)
	action, err := aicommon.ExtractAction(
		`{"@action":"directly_answer","answer_payload":"最终结论"}`,
		"directly_answer",
	)
	require.NoError(t, err)
	require.NoError(t, loopActionDirectlyAnswerHTTPFlowAnalyze.ActionVerifier(loop, action))

	op := reactloops.NewActionHandlerOperator(task)
	loopActionDirectlyAnswerHTTPFlowAnalyze.ActionHandler(loop, action, op)

	require.True(t, op.IsContinued())
	terminated, termErr := op.IsTerminated()
	require.False(t, terminated)
	require.NoError(t, termErr)
	require.Len(t, invoker.results, 1)
	assert.Equal(t, "最终结论", invoker.results[0])
}
