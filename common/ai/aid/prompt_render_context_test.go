package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type promptRenderLoopStub struct {
	iteration int
}

func (s *promptRenderLoopStub) Execute(taskId string, ctx context.Context, userInput string) error {
	return nil
}

func (s *promptRenderLoopStub) ExecuteWithExistedTask(task aicommon.AIStatefulTask) error {
	return nil
}

func (s *promptRenderLoopStub) GetCurrentTask() aicommon.AIStatefulTask {
	return nil
}

func (s *promptRenderLoopStub) SetCurrentTask(t aicommon.AIStatefulTask) {}

func (s *promptRenderLoopStub) GetInvoker() aicommon.AIInvokeRuntime {
	return nil
}

func (s *promptRenderLoopStub) GetEmitter() *aicommon.Emitter {
	return nil
}

func (s *promptRenderLoopStub) GetConfig() aicommon.AICallerConfigIf {
	return nil
}

func (s *promptRenderLoopStub) GetMemoryTriage() aicommon.MemoryTriage {
	return nil
}

func (s *promptRenderLoopStub) GetEnableSelfReflection() bool {
	return false
}

func (s *promptRenderLoopStub) Set(key string, value any) {}

func (s *promptRenderLoopStub) Get(key string) string {
	return ""
}

func (s *promptRenderLoopStub) GetVariable(key string) any {
	return nil
}

func (s *promptRenderLoopStub) GetStringSlice(key string) []string {
	return nil
}

func (s *promptRenderLoopStub) GetInt(key string) int {
	return 0
}

func (s *promptRenderLoopStub) RemoveAction(actionType string) {}

func (s *promptRenderLoopStub) GetAllActionNames() []string {
	return nil
}

func (s *promptRenderLoopStub) NoActions() bool {
	return true
}

func (s *promptRenderLoopStub) PushMemory(result *aicommon.SearchMemoryResult) {}

func (s *promptRenderLoopStub) GetCurrentMemoriesContent() string {
	return ""
}

func (s *promptRenderLoopStub) DisallowAskForClarification() {}

func (s *promptRenderLoopStub) GetTimelineDiff() (string, error) {
	return "", nil
}

func (s *promptRenderLoopStub) GetCurrentIterationIndex() int {
	return s.iteration
}

type promptRenderFixture struct {
	coordinator *Coordinator
	provider    *PromptContextProvider
	root        *AiTask
	taskA       *AiTask
	taskB       *AiTask
}

func newPromptRenderFixture() *promptRenderFixture {
	provider := GetDefaultContextProvider()
	provider.StoreToolsKeywords(func() []string { return []string{"grep", "read_file"} })
	coordinator := &Coordinator{
		Config: &aicommon.Config{
			Ctx:             context.Background(),
			MaxTaskContinue: 3,
		},
		ContextProvider: provider,
		userInput:       "prompt render regression",
	}

	root := coordinator.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	root.SetUserInput("root input\n\n<EVIDENCE>\nA evidence marker\n</EVIDENCE>")

	taskA := coordinator.generateAITaskWithName("Task A", "goal from task A")
	taskA.Index = "1-1"
	taskA.ParentTask = root
	taskA.SetUserInput("input-from-task-A")
	taskA.StatusSummary = "status from task A"
	root.Subtasks = []*AiTask{taskA}

	taskB := coordinator.generateAITaskWithName("Task B", "goal from task B")
	taskB.Index = "2-1"
	taskB.SetUserInput("input-from-task-B\n\n<EVIDENCE>\nB evidence marker\n</EVIDENCE>")
	taskB.StatusSummary = "status from task B"
	taskB.SetReActLoop(&promptRenderLoopStub{iteration: 99})

	coordinator.rootTask = root
	provider.StoreRootTask(root)
	provider.StoreCurrentTask(taskB)

	resultA := &aitool.ToolResult{ID: 101, Name: "alpha_tool", Param: map[string]any{"q": "alpha"}, Success: true}
	resultB := &aitool.ToolResult{ID: 202, Name: "beta_tool", Param: map[string]any{"q": "beta"}, Success: true}
	taskA.PushToolCallResult(resultA)
	taskB.PushToolCallResult(resultB)
	provider.PushToolCallResults(resultA)
	provider.PushToolCallResults(resultB)

	return &promptRenderFixture{
		coordinator: coordinator,
		provider:    provider,
		root:        root,
		taskA:       taskA,
		taskB:       taskB,
	}
}

func TestRenderCurrentTaskInfo_UsesExplicitTaskContext(t *testing.T) {
	fixture := newPromptRenderFixture()

	out := fixture.provider.RenderCurrentTaskInfo(fixture.taskA)

	require.Contains(t, out, "--- 当前任务 ---")
	require.Contains(t, out, "input-from-task-A")
	require.Contains(t, out, "A evidence marker")
	require.Contains(t, out, "status from task A")
	require.NotContains(t, out, "B evidence marker")
	require.NotContains(t, out, "input-from-task-B")
	require.NotContains(t, out, "status from task B")
}

func TestGenerateDeepThinkPlanPrompt_UsesTaskLocalGoal(t *testing.T) {
	fixture := newPromptRenderFixture()

	prompt, err := fixture.taskA.GenerateDeepThinkPlanPrompt("go deeper")
	require.NoError(t, err)

	require.Contains(t, prompt, "## 当前步骤任务目标\ngoal from task A")
	require.NotContains(t, prompt, "## 当前步骤任务目标\ngoal from task B")
}

func TestGenerateToolCallResponsePrompt_UsesTaskLocalContinueState(t *testing.T) {
	fixture := newPromptRenderFixture()

	prompt, err := fixture.taskA.generateToolCallResponsePrompt(
		&aitool.ToolResult{ID: 303, Name: "result_tool", Success: true, Data: map[string]any{"ok": true}},
		aitool.NewWithoutCallback("test_tool", aitool.WithDescription("test tool")),
	)
	require.NoError(t, err)

	require.Contains(t, prompt, "当前任务可以继续")
	require.NotContains(t, prompt, "当前任务已经超过了最大执行次数")
	require.Contains(t, prompt, "--- 当前任务 ---")
	require.Contains(t, prompt, "input-from-task-A")
	require.NotContains(t, prompt, "input-from-task-B")
}

func TestGenerateTaskSummaryPrompt_UsesTaskLocalTimeline(t *testing.T) {
	fixture := newPromptRenderFixture()

	prompt, err := fixture.taskA.GenerateTaskSummaryPrompt()
	require.NoError(t, err)

	require.Contains(t, prompt, "## 当前任务的历史时间线")
	require.Contains(t, prompt, "alpha_tool")
	require.NotContains(t, prompt, "beta_tool")
	require.True(t, strings.Contains(prompt, "input-from-task-A"))
}
