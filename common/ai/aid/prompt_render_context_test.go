package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

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
			Timeline:        provider.timeline,
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

	currentTask := aicommon_testutil.MustExtractAITagBlock(t, prompt, "CURRENT_TASK").Body
	require.Contains(t, currentTask, "input-from-task-A")
	require.NotContains(t, currentTask, "input-from-task-B")
}

func TestGenerateTaskSummaryPrompt_Timeline(t *testing.T) {
	fixture := newPromptRenderFixture()

	prompt, err := fixture.taskA.GenerateTaskSummaryPrompt()
	require.NoError(t, err)

	require.Contains(t, prompt, "alpha_tool")
	require.True(t, strings.Contains(prompt, "input-from-task-A"))
	require.Contains(t, prompt, "工具调用是否结束、工具执行是否成功、用户任务是否满足")
	require.Contains(t, prompt, "所有未终结的 TODO")
}
