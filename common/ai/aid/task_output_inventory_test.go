package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestTaskOutputInventoryContextIncludesDiscoveredURLs(t *testing.T) {
	provider := GetDefaultContextProvider()
	coordinator := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: provider,
	}
	task := coordinator.generateAITaskWithName("收集接口", "待测列表：\n- GET /user/id?id=1\n\n验收标准：\n- 输出接口清单")
	task.Index = "1-1"
	task.TaskSummary = "发现了多个真实接口"
	task.SetStatus(aicommon.AITaskState_Completed)
	task.PushToolCallResult(&aitool.ToolResult{
		ID:      1,
		Name:    "simple_crawler",
		Success: true,
		Data: &aitool.ToolExecutionResult{
			Stdout: strings.Join([]string{
				"[GET] http://127.0.0.1:8080/user/id?id=1",
				"[GET] http://127.0.0.1:8080/user/name?name=admin",
			}, "\n"),
		},
	})

	provider.RegisterTaskOutputSnapshot(task, "/tmp/task_1-1_collect", "/tmp/task_1-1_collect/task_1_1_collect_result_summary.txt")

	contextText := provider.TaskOutputInventoryContext()
	require.Contains(t, contextText, taskOutputInventoryPersistentKey)
	require.Contains(t, contextText, "http://127.0.0.1:8080/user/id?id=1")
	require.Contains(t, contextText, "http://127.0.0.1:8080/user/name?name=admin")
	require.Contains(t, contextText, "task_1_1_collect_result_summary.txt")

	targets := provider.TaskOutputInventoryTargets()
	require.Contains(t, targets, "GET /user/id?id=1")
	require.Contains(t, targets, "http://127.0.0.1:8080/user/id?id=1")
	require.Contains(t, targets, "http://127.0.0.1:8080/user/name?name=admin")
}

func TestCollectInventoryCoverageGapsDetectsUnassignedTargets(t *testing.T) {
	provider := GetDefaultContextProvider()
	coordinator := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: provider,
	}
	provider.UpsertTaskOutputInventoryEntry(&taskOutputInventoryEntry{
		TaskIndex:      "1-1",
		TaskName:       "收集靶场接口",
		DiscoveredURLs: []string{"http://127.0.0.1:8080/user/id?id=1", "http://127.0.0.1:8080/user/name?name=admin"},
	})

	root := coordinator.generateAITaskWithName("根任务", "总目标")
	child := coordinator.generateAITaskWithName("验证 SQL 注入 group 1", "待测列表：\n- GET /user/id?id=1\n\n验收标准：\n- 输出验证结论")
	child.ParentTask = root
	root.Subtasks = []*AiTask{child}

	pr := &planRequest{cod: coordinator}
	missing := pr.collectInventoryCoverageGaps(root)
	require.Contains(t, missing, "http://127.0.0.1:8080/user/name?name=admin")
	require.NotContains(t, missing, "http://127.0.0.1:8080/user/id?id=1")

	root = appendInventoryCoverageTasks(root, missing)
	require.Len(t, root.Subtasks, 2)
	require.Contains(t, root.Subtasks[1].Goal, "http://127.0.0.1:8080/user/name?name=admin")
	require.Contains(t, root.Subtasks[1].Goal, taskOutputInventoryPersistentKey)
}
