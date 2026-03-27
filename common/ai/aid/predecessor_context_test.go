package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestFindTaskByIndex(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	child1 := cod.generateAITaskWithName("Child1", "goal1")
	child1.Index = "1-1"
	child2 := cod.generateAITaskWithName("Child2", "goal2")
	child2.Index = "1-2"
	grandchild := cod.generateAITaskWithName("Grandchild", "goal gc")
	grandchild.Index = "1-2-1"
	child2.Subtasks = []*AiTask{grandchild}
	root.Subtasks = []*AiTask{child1, child2}

	require.Equal(t, root, findTaskByIndex(root, "1"))
	require.Equal(t, child1, findTaskByIndex(root, "1-1"))
	require.Equal(t, child2, findTaskByIndex(root, "1-2"))
	require.Equal(t, grandchild, findTaskByIndex(root, "1-2-1"))
	require.Nil(t, findTaskByIndex(root, "1-3"))
	require.Nil(t, findTaskByIndex(nil, "1"))
	require.Nil(t, findTaskByIndex(root, ""))
}

func TestPredecessorTasksContext_CompletedSiblings(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	task1 := cod.generateAITaskWithName("Scan Ports", "scan all ports")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.ShortSummary = "Found 26 open ports on target"
	task1.SetStatus(aicommon.AITaskState_Completed)

	task2 := cod.generateAITaskWithName("Enumerate Services", "identify services")
	task2.Index = "1-2"
	task2.ParentTask = root
	task2.ShortSummary = "Identified Nginx 1.18.0 and Tomcat 8.5.65"
	task2.SetStatus(aicommon.AITaskState_Completed)

	task3 := cod.generateAITaskWithName("Verify Vulnerabilities", "check vulns")
	task3.Index = "1-3"
	task3.ParentTask = root

	root.Subtasks = []*AiTask{task1, task2, task3}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task3
	mem.RootTask = root

	result := mem.PredecessorTasksContext()

	require.Contains(t, result, "[1-1]")
	require.Contains(t, result, "Scan Ports")
	require.Contains(t, result, "Found 26 open ports")
	require.Contains(t, result, "[1-2]")
	require.Contains(t, result, "Enumerate Services")
	require.Contains(t, result, "Nginx 1.18.0")
	require.Contains(t, result, "task_1_1_")
	require.Contains(t, result, "task_1_2_")
	require.Contains(t, result, "result_summary")
}

func TestPredecessorTasksContext_DependsOn(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cod.rootTask = root

	task1 := cod.generateAITaskWithName("Gather Info", "gather info")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.LongSummary = "Source directory is /opt/app/src, sink file at /tmp/output.log"
	task1.SetStatus(aicommon.AITaskState_Completed)

	task2 := cod.generateAITaskWithName("Another Task", "another")
	task2.Index = "1-2"
	task2.ParentTask = root
	task2.SetStatus(aicommon.AITaskState_Completed)
	task2.ShortSummary = "Config file at /etc/app.conf"

	task3 := cod.generateAITaskWithName("Verify Result", "verify")
	task3.Index = "1-3"
	task3.ParentTask = root
	task3.DependsOn = []string{"1-1"}

	root.Subtasks = []*AiTask{task1, task2, task3}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task3
	mem.RootTask = root

	result := mem.PredecessorTasksContext()

	require.Contains(t, result, "[1-1]")
	require.Contains(t, result, "Gather Info")
	require.Contains(t, result, "/opt/app/src")

	require.Contains(t, result, "[1-2]")
	require.Contains(t, result, "Another Task")
}

func TestPredecessorTasksContext_DependsOnNonSibling(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cod.rootTask = root

	task1 := cod.generateAITaskWithName("Deep Task", "deep")
	task1.Index = "1-1-1"
	task1.ShortSummary = "Located binary at /usr/local/bin/app"
	task1.SetStatus(aicommon.AITaskState_Completed)

	group1 := cod.generateAITaskWithName("Group1", "group")
	group1.Index = "1-1"
	group1.ParentTask = root
	group1.Subtasks = []*AiTask{task1}
	task1.ParentTask = group1

	task2 := cod.generateAITaskWithName("Verify", "verify")
	task2.Index = "1-2"
	task2.ParentTask = root
	task2.DependsOn = []string{"1-1-1"}

	root.Subtasks = []*AiTask{group1, task2}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task2
	mem.RootTask = root

	result := mem.PredecessorTasksContext()

	require.Contains(t, result, "[1-1-1]")
	require.Contains(t, result, "Deep Task")
	require.Contains(t, result, "/usr/local/bin/app")
}

func TestPredecessorTasksContext_NoPredecessors(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	task1 := cod.generateAITaskWithName("First Task", "first")
	task1.Index = "1-1"
	task1.ParentTask = root

	root.Subtasks = []*AiTask{task1}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task1
	mem.RootTask = root

	result := mem.PredecessorTasksContext()
	require.Empty(t, result)
}

func TestPredecessorTasksContext_NilCurrentTask(t *testing.T) {
	mem := GetDefaultContextProvider()
	mem.CurrentTask = nil

	result := mem.PredecessorTasksContext()
	require.Empty(t, result)
}

func TestPredecessorTasksContext_SummaryTruncation(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	longSummary := strings.Repeat("A", 300)

	task1 := cod.generateAITaskWithName("Long Summary Task", "long")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.ShortSummary = longSummary
	task1.SetStatus(aicommon.AITaskState_Completed)

	task2 := cod.generateAITaskWithName("Current", "current")
	task2.Index = "1-2"
	task2.ParentTask = root

	root.Subtasks = []*AiTask{task1, task2}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task2
	mem.RootTask = root

	result := mem.PredecessorTasksContext()

	require.Contains(t, result, "...")
	require.Less(t, len(result), 400)
}

func TestPredecessorTasksContext_DependsOnDedup(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cod.rootTask = root

	task1 := cod.generateAITaskWithName("Task A", "a")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.ShortSummary = "Result A"
	task1.SetStatus(aicommon.AITaskState_Completed)

	task2 := cod.generateAITaskWithName("Task B", "b")
	task2.Index = "1-2"
	task2.ParentTask = root
	task2.DependsOn = []string{"1-1"}

	root.Subtasks = []*AiTask{task1, task2}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task2
	mem.RootTask = root

	result := mem.PredecessorTasksContext()

	count := strings.Count(result, "[1-1]")
	require.Equal(t, 1, count, "sibling 1-1 already collected, DependsOn should not duplicate it")
}

func TestPredecessorTasksContext_SkipsIncomplete(t *testing.T) {
	cod := &Coordinator{
		Config:    &aicommon.Config{Ctx: context.Background()},
		userInput: "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"

	task1 := cod.generateAITaskWithName("Processing Task", "processing")
	task1.Index = "1-1"
	task1.ParentTask = root
	task1.ShortSummary = "Still running"
	task1.SetStatus(aicommon.AITaskState_Processing)

	task2 := cod.generateAITaskWithName("Current", "current")
	task2.Index = "1-2"
	task2.ParentTask = root

	root.Subtasks = []*AiTask{task1, task2}

	mem := GetDefaultContextProvider()
	mem.CurrentTask = task2
	mem.RootTask = root

	result := mem.PredecessorTasksContext()
	require.Empty(t, result)
}
