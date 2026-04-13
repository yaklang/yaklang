package aid

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestSharedEvidenceContext_UsesPersistentEvidence(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cur := cod.generateAITaskWithName("Current", "current goal")
	cur.Index = "1-1"
	cur.ParentTask = root
	root.Subtasks = []*AiTask{cur}

	mem.CurrentTask = cur
	mem.RootTask = root
	mem.SetPersistentData(planEvidencePersistentKey, "## HTTP 观察\n- GET /health 返回 200 OK")

	result := mem.SharedEvidenceContext()
	require.Equal(t, "## HTTP 观察\n- GET /health 返回 200 OK", result)
}

func TestSharedEvidenceContext_NoCurrentTask(t *testing.T) {
	mem := GetDefaultContextProvider()
	require.Empty(t, mem.SharedEvidenceContext())
}

func TestSharedEvidenceContext_TruncatesLongEvidence(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config:          &aicommon.Config{Ctx: context.Background()},
		ContextProvider: mem,
		userInput:       "test",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cur := cod.generateAITaskWithName("Current", "current goal")
	cur.Index = "1-1"
	cur.ParentTask = root
	root.Subtasks = []*AiTask{cur}

	mem.CurrentTask = cur
	mem.RootTask = root
	mem.SetPersistentData(planEvidencePersistentKey, "## Evidence\n- "+strings.Repeat("A", 2000))

	result := mem.SharedEvidenceContext()
	require.Contains(t, result, "...")
	require.Less(t, len([]rune(result)), 1700)
}

const (
	peTaskMarkerSharedEvidence = "--- 共享执行证据 ---"
	peTaskMarkerPrecondition   = "--- 先决条件检查 ---"
	peTaskMarkerToolHint       = "find_file、grep、read_file"
)

func TestCurrentTaskInfo_PeTaskPrompt_EndToEnd_WithEvidence(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config: &aicommon.Config{
			Ctx:             context.Background(),
			MaxTaskContinue: 10,
		},
		ContextProvider: mem,
		userInput:       "e2e user query",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	cur := cod.generateAITaskWithName("Current Task", "current goal")
	cur.Index = "1-1"
	cur.ParentTask = root
	root.Subtasks = []*AiTask{cur}

	mem.CurrentTask = cur
	mem.RootTask = root
	mem.SetPersistentData(planEvidencePersistentKey, "## 文件变更\n- /tmp/report.md 已写入")

	out := mem.CurrentTaskInfo()

	require.Contains(t, out, peTaskMarkerSharedEvidence)
	require.Contains(t, out, peTaskMarkerPrecondition)
	require.Contains(t, out, peTaskMarkerToolHint)
	require.Contains(t, out, "/tmp/report.md 已写入")
	idx := strings.Index(out, peTaskMarkerSharedEvidence)
	pre := strings.Index(out, peTaskMarkerPrecondition)
	require.Greater(t, pre, idx)
	require.NotContains(t, out, "前序任务交付索引")
}

func TestCurrentTaskInfo_PeTaskPrompt_EndToEnd_NoEvidence(t *testing.T) {
	mem := GetDefaultContextProvider()
	cod := &Coordinator{
		Config: &aicommon.Config{
			Ctx:             context.Background(),
			MaxTaskContinue: 10,
		},
		ContextProvider: mem,
		userInput:       "e2e no evidence",
	}

	root := cod.generateAITaskWithName("Root", "root goal")
	root.Index = "1"
	first := cod.generateAITaskWithName("Only Task", "only goal")
	first.Index = "1-1"
	first.ParentTask = root
	root.Subtasks = []*AiTask{first}

	mem.CurrentTask = first
	mem.RootTask = root

	out := mem.CurrentTaskInfo()

	require.NotContains(t, out, peTaskMarkerSharedEvidence)
	require.Contains(t, out, peTaskMarkerPrecondition)
	require.Contains(t, out, peTaskMarkerToolHint)
}
