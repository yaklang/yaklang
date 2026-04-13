package aireact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestRenderVerificationTodoSnapshot_AggregatesStatuses(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "collect_signal", Content: "收集页面响应信号"},
				{Op: "add", ID: "fix_title", Content: "修正标题"},
			},
		},
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "done", ID: "collect_signal"},
				{Op: "delete", ID: "fix_title"},
				{Op: "add", ID: "replay_payload", Content: "使用新 payload 复测"},
			},
		},
		{
			Satisfied:     true,
			NextMovements: []aicommon.VerifyNextMovement{},
		},
	}

	snapshot := renderVerificationTodoSnapshot(history)
	require.Contains(t, snapshot, "- [x]: [id: collect_signal]: 收集页面响应信号")
	require.Contains(t, snapshot, "- [DELETED]: [id: fix_title]: 修正标题")
	require.Contains(t, snapshot, "- [SKIPPED]: [id: replay_payload]: 使用新 payload 复测")
}

func TestRenderVerificationTodoSnapshot_PrioritizesActiveItemsUnderLimit(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{}
	for index := 0; index < 400; index++ {
		history = append(history, &aicommon.VerifySatisfactionResult{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{
					Op:      "add",
					ID:      fmt.Sprintf("todo-%03d-%s-%c-%s", index, strings.Repeat("x", 20), rune('a'+(index%26)), strings.Repeat("z", 20)),
					Content: strings.Repeat("非常长的待办描述", 30),
				},
			},
		})
	}
	history = append(history, &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "active_focus", Content: "优先保留这个活跃 TODO"},
		},
	})

	snapshot := renderVerificationTodoSnapshot(history)
	require.LessOrEqual(t, aicommon.MeasureTokens(snapshot), verificationTodoSnapshotLimit)
	require.Contains(t, snapshot, "active_focus")
	require.Contains(t, snapshot, "TODO history exceeded 10K tokens")
}

func TestBuildVerificationTodoItems_DoneKeepsLatestContent(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "rename_file", Content: "先创建临时文件"},
			},
		},
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "rename_file", Content: "重命名临时文件为最终名称"},
				{Op: "done", ID: "rename_file"},
			},
		},
	}

	items := buildVerificationTodoItems(history)
	require.Len(t, items, 1)
	require.Equal(t, verificationTodoStatusDone, items[0].Status)
	require.Equal(t, "重命名临时文件为最终名称", items[0].Content)
}

func TestBuildVerificationTodoItems_DeleteKeepsLatestContentAndStats(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "obsolete_step", Content: "旧的验证步骤"},
			},
		},
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "obsolete_step", Content: "不再需要的验证步骤"},
				{Op: "delete", ID: "obsolete_step"},
			},
		},
	}

	items, stats := buildVerificationTodoItemsAndStats(history)
	require.Len(t, items, 1)
	require.Equal(t, verificationTodoStatusDeleted, items[0].Status)
	require.Equal(t, "不再需要的验证步骤", items[0].Content)
	require.Equal(t, 1, stats.Deleted)
	require.Zero(t, stats.Done)
}

func TestBuildVerificationTodoItems_DeleteCanBeReactivated(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{
		{
			Satisfied:     false,
			NextMovements: []aicommon.VerifyNextMovement{{Op: "add", ID: "retry_payload", Content: "第一次 payload"}},
		},
		{
			Satisfied:     false,
			NextMovements: []aicommon.VerifyNextMovement{{Op: "delete", ID: "retry_payload"}},
		},
		{
			Satisfied:     false,
			NextMovements: []aicommon.VerifyNextMovement{{Op: "add", ID: "retry_payload", Content: "重新激活 payload"}},
		},
	}

	items := buildVerificationTodoItems(history)
	require.Len(t, items, 1)
	require.Equal(t, verificationTodoStatusPending, items[0].Status)
	require.Equal(t, "重新激活 payload", items[0].Content)
}

func TestRenderVerificationTodoMarkdownSnapshot_AppliesCurrentDeltaMarkers(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "add", ID: "old_pending", Content: "这是一额个未完成的旧任务"},
				{Op: "add", ID: "old_done", Content: "这是一个已经完成的旧任务"},
			},
		},
		{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{Op: "done", ID: "old_done"},
			},
		},
	}

	current := &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "doing", ID: "old_done"},
			{Op: "done", ID: "old_pending"},
			{Op: "delete", ID: "old_delete_target"},
			{Op: "add", ID: "new_task", Content: "这是一个新增的任务"},
		},
	}
	history[0].NextMovements = append(history[0].NextMovements, aicommon.VerifyNextMovement{Op: "add", ID: "old_delete_target", Content: "这是一个会被删除的任务"})

	snapshot := renderVerificationTodoMarkdownSnapshot(history, current)
	require.Contains(t, snapshot, "- [ ] (doing) 这是一个已经完成的旧任务")
	require.Contains(t, snapshot, "- [ ] (new) 这是一个新增的任务")
	require.Contains(t, snapshot, "- [x] (done) ~~这是一额个未完成的旧任务~~")
	require.Contains(t, snapshot, "- [x] (deleted) ~~这是一个会被删除的任务~~")
}

func TestSanitizeVerificationTodoMarkdownContent_PreventsLineBreakInjection(t *testing.T) {
	content := sanitizeVerificationTodoMarkdownContent("第一行\n- [x]: 注入内容\t第二段")
	require.Equal(t, "第一行 - [x]: 注入内容 第二段", content)
	require.NotContains(t, content, "\n")
	require.NotContains(t, content, "\r")
}
