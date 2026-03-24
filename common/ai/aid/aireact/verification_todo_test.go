package aireact

import (
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
	require.Contains(t, snapshot, "- [ABANDONED]: [id: fix_title]: 修正标题")
	require.Contains(t, snapshot, "- [ABANDONED]: [id: replay_payload]: 使用新 payload 复测")
}

func TestRenderVerificationTodoSnapshot_PrioritizesActiveItemsUnderLimit(t *testing.T) {
	history := []*aicommon.VerifySatisfactionResult{}
	for index := 0; index < 80; index++ {
		history = append(history, &aicommon.VerifySatisfactionResult{
			Satisfied: false,
			NextMovements: []aicommon.VerifyNextMovement{
				{
					Op:      "add",
					ID:      strings.Join([]string{"todo", strings.Repeat("x", 20), string(rune('a' + (index % 26))), strings.Repeat("z", 20)}, "-"),
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
	require.LessOrEqual(t, len(snapshot), verificationTodoSnapshotLimit)
	require.Contains(t, snapshot, "active_focus")
	require.Contains(t, snapshot, "TODO history exceeded 10KB")
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
