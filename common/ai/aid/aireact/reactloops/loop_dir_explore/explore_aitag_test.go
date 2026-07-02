package loop_dir_explore

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestMergeLoopActionToolParams_mergesContentAITAG(t *testing.T) {
	action, err := aicommon.ExtractActionFromStream(
		context.Background(),
		strings.NewReader(`{"@action":"write_file","file":"/tmp/dir_structure.md","force":true}`),
		"object",
	)
	require.NoError(t, err)
	action.WaitParse(context.Background())
	action.WaitStream(context.Background())
	action.ForceSet(aicommon.GetToolParamAITagActionKey("content"), "# dirs\n/foo/\n/bar/\n")

	params := reactloops.MergeLoopActionToolParams(action, action.GetParams(), []string{"content"})
	require.Equal(t, "# dirs\n/foo/\n/bar/\n", params.GetString("content"))
}

func TestToolParamAITagNames_filtersUnsupportedNames(t *testing.T) {
	names := aicommon.FilterSupportedToolParamAITagNames([]string{"file", "content", "raw-content"})
	require.Contains(t, names, "content")
	require.Contains(t, names, "file")
	require.NotContains(t, names, "raw-content")
}

func TestWriteFileAction_extractAITAGContent_endToEnd(t *testing.T) {
	nonce := "ab12"
	raw := `{
  "@action": "write_file",
  "identifier": "write_dir_structure",
  "human_readable_thought": "目录树已完整获取，立即写入 dir_structure.md。",
  "file": "/Users/zwh_china/yakit-projects/aispace/122_irify_audit_scan_20260626_bab54/audit/dir_structure.md",
  "force": true,
  "next_movements": [
    {
      "op": "add",
      "id": "find_entry_points",
      "content": "搜索项目所有入口点（main 函数）"
    },
    {
      "op": "doing",
      "id": "explore_dir_structure"
    }
  ]
}
<|TOOL_PARAM_content_` + nonce + `|>
# Directory Structure

.github/
cmd/
internal/
<|TOOL_PARAM_content_END_` + nonce + `|>`

	opts := []aicommon.ActionMakerOption{
		aicommon.WithActionNonce(nonce),
		aicommon.WithActionTagToKeyAndExtraNonces(
			"TOOL_PARAM_content",
			aicommon.GetToolParamAITagActionKey("content"),
			aicommon.LiteralCurrentNoncePlaceholder,
			aicommon.RecentToolCacheStableNonce,
		),
	}

	action, err := aicommon.ExtractActionFromStream(
		context.Background(),
		strings.NewReader(raw),
		"object",
		opts...,
	)
	require.NoError(t, err)
	require.NotNil(t, action)
	action.WaitParse(context.Background())
	action.WaitStream(context.Background())

	params := reactloops.MergeLoopActionToolParams(action, action.GetParams(), []string{"content"})
	content := params.GetString("content")
	require.Contains(t, content, "# Directory Structure")
	require.Contains(t, content, "internal/")
}
