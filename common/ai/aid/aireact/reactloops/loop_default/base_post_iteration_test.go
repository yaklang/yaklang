package loop_default

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

// postIterationTestInvoker 记录 post-iteration 总结钩子对 invoker 的调用，
// 用于断言最终总结是否生成。
type postIterationTestInvoker struct {
	*mock.MockInvoker

	mu                  sync.Mutex
	directAnswerQueries []string
	timelineEntries     map[string][]string
}

func newPostIterationTestInvoker(cb aicommon.AICallbackType) *postIterationTestInvoker {
	ctx := context.Background()
	base := mock.NewMockInvoker(ctx)
	base.SetConfig(aicommon.NewConfig(
		ctx,
		aicommon.WithAICallback(cb),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
	))
	return &postIterationTestInvoker{
		MockInvoker:     base,
		timelineEntries: make(map[string][]string),
	}
}

func (i *postIterationTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.directAnswerQueries = append(i.directAnswerQueries, query)
	return "mocked final summary", nil
}

func (i *postIterationTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timelineEntries[entry] = append(i.timelineEntries[entry], content)
}

func (i *postIterationTestInvoker) directAnswerCalls() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.directAnswerQueries)
}

func (i *postIterationTestInvoker) timelineValues(entry string) []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.timelineEntries[entry]...)
}

// newPostIterationTestLoop 通过注册的 default loop 工厂构建循环，
// 关闭与本次断言无关的能力，只保留 post-iteration 总结钩子的行为。
func newPostIterationTestLoop(t *testing.T, invoker *postIterationTestInvoker, extraOpts ...reactloops.ReActLoopOption) *reactloops.ReActLoop {
	t.Helper()
	opts := []reactloops.ReActLoopOption{
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
	}
	opts = append(opts, extraOpts...)
	loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_DEFAULT, invoker, opts...)
	require.NoError(t, err)
	return loop
}

// scriptedCallback 按调用次序依次返回脚本化的 AI 响应。
// finish 动作首次触发软 TODO 检查点会继续迭代，因此脚本需要两次 finish 才会退出。
func scriptedCallback(responses ...string) aicommon.AICallbackType {
	var mu sync.Mutex
	callCount := 0
	return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		mu.Lock()
		idx := callCount
		callCount++
		mu.Unlock()
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(responses[idx]))
		rsp.Close()
		return rsp, nil
	}
}

// TestDefaultLoop_PostIterationSkipsSummaryAfterDirectlyAnswer 是本次改动的回归测试：
// 循环以 directly_answer -> finish -> finish 收尾时，post-iteration 钩子必须通过
// GetLastValidAction 看到 finish 之前的 directly_answer，从而跳过最终总结。
// 旧实现使用 GetLastAction 只会看到 finish 记录，导致直接回答后仍多余地生成总结。
func TestDefaultLoop_PostIterationSkipsSummaryAfterDirectlyAnswer(t *testing.T) {
	invoker := newPostIterationTestInvoker(scriptedCallback(
		`{"@action": "directly_answer", "answer_payload": "最终答案"}`,
		`{"@action": "finish", "answer": "done"}`,
		`{"@action": "finish", "answer": "done"}`,
	))
	loop := newPostIterationTestLoop(t, invoker)

	err := loop.Execute("post-iteration-skip-task", context.Background(), "回答一个问题")
	require.NoError(t, err)

	require.Equal(t, 0, invoker.directAnswerCalls(),
		"last valid action is directly_answer; post-iteration summary must be skipped")
	require.Empty(t, invoker.timelineValues("final_summary"),
		"no final_summary timeline entry should be recorded after directly_answer")

	lastValid := loop.GetLastValidAction()
	require.NotNil(t, lastValid)
	require.Equal(t, schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER, lastValid.ActionType)
}

// TestDefaultLoop_PostIterationGeneratesSummaryAfterOtherAction 是对照场景：
// 收尾前最后一次有效 action 不是 directly_answer 时，post-iteration 钩子必须
// 调用 invoker.DirectlyAnswer 生成最终总结，并写入 final_summary 时间线。
func TestDefaultLoop_PostIterationGeneratesSummaryAfterOtherAction(t *testing.T) {
	invoker := newPostIterationTestInvoker(scriptedCallback(
		`{"@action": "record_note", "note": "做了一些工作"}`,
		`{"@action": "finish", "answer": "done"}`,
		`{"@action": "finish", "answer": "done"}`,
	))
	loop := newPostIterationTestLoop(t, invoker,
		reactloops.WithRegisterLoopAction(
			"record_note",
			"Record a note for testing",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Continue()
			},
		),
	)

	err := loop.Execute("post-iteration-summary-task", context.Background(), "执行一个任务")
	require.NoError(t, err)

	require.Equal(t, 1, invoker.directAnswerCalls(),
		"last valid action is not directly_answer; post-iteration summary must be generated")
	require.Equal(t, reActPostSummary, invoker.directAnswerQueries[0],
		"summary must use the post-summary prompt")
	require.Contains(t, invoker.timelineValues("final_summary"), "mocked final summary",
		"summary result must be recorded to the final_summary timeline entry")
}
