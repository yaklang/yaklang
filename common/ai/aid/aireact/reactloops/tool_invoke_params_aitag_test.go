package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const (
	sampleWriteFileNonce      = "ab12"
	sampleTodoContent         = "搜索项目所有入口点（main 函数）"
	sampleDirStructureContent = `# 目录结构

.github/
  workflows/
build/
cmd/
  answer/
internal/
  base/
  controller/
  repo/
  service/
pkg/
ui/
  src/
`
)

// sampleWriteFileWithNextMovementsAIResponse reproduces the problematic dir_explore
// model output: write_file JSON carries next_movements, while the real file body
// lives in a TOOL_PARAM_content AITAG block outside JSON.
func sampleWriteFileWithNextMovementsAIResponse(nonce string) string {
	return `{
  "@action": "write_file",
  "identifier": "write_dir_structure",
  "human_readable_thought": "目录树已完整获取，立即写入 dir_structure.md。",
  "file": "/Users/zwh_china/yakit-projects/aispace/122_irify_audit_scan_20260626_bab54/audit/dir_structure.md",
  "force": true,
  "next_movements": [
    {
      "op": "add",
      "id": "find_entry_points",
      "content": "` + sampleTodoContent + `"
    },
    {
      "op": "doing",
      "id": "explore_dir_structure"
    }
  ]
}
<|TOOL_PARAM_content_` + nonce + `|>
` + sampleDirStructureContent + `<|TOOL_PARAM_content_END_` + nonce + `|>`
}

func toolParamContentAITagOptions(nonce string) []aicommon.ActionMakerOption {
	return []aicommon.ActionMakerOption{
		aicommon.WithActionNonce(nonce),
		aicommon.WithActionTagToKeyAndExtraNonces(
			"TOOL_PARAM_content",
			aicommon.GetToolParamAITagActionKey("content"),
			aicommon.LiteralCurrentNoncePlaceholder,
			aicommon.RecentToolCacheStableNonce,
		),
	}
}

func parseSampleWriteFileAction(t *testing.T, raw string, opts ...aicommon.ActionMakerOption) *aicommon.Action {
	t.Helper()
	opts = append([]aicommon.ActionMakerOption{
		aicommon.WithActionAlias("object", "write_file"),
	}, opts...)
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
	require.Equal(t, "write_file", action.ActionType())
	return action
}

func TestParseWriteFileWithNextMovements_withoutAITAGRegistration(t *testing.T) {
	raw := sampleWriteFileWithNextMovementsAIResponse(sampleWriteFileNonce)
	action := parseSampleWriteFileAction(t, raw)

	require.Equal(t, sampleTodoContent, action.GetString("content"),
		"flat content should be polluted by next_movements[].content")

	aitagKey := aicommon.GetToolParamAITagActionKey("content")
	require.Empty(t, action.GetString(aitagKey),
		"without TOOL_PARAM_content registration the directory tree AITAG is dropped")

	nested := action.GetParams()
	require.NotEmpty(t, nested.GetString("file"))
	require.True(t, nested.GetBool("force"))
	require.Empty(t, nested.GetString("content"),
		"GetParams() must not contain write_file body when it only exists in AITAG")

	toolParams := action.GetParams()
	require.Empty(t, toolParams.GetString("content"),
		"write_file tool would fail validation: missing property 'content'")

	merged := MergeLoopActionToolParams(action, action.GetParams(), []string{"content"})
	require.Empty(t, merged.GetString("content"),
		"MergeLoopActionToolParams cannot invent content when AITAG was never parsed")
}

func TestParseWriteFileWithNextMovements_withAITAGRegistrationAndMerge(t *testing.T) {
	raw := sampleWriteFileWithNextMovementsAIResponse(sampleWriteFileNonce)
	action := parseSampleWriteFileAction(t, raw, toolParamContentAITagOptions(sampleWriteFileNonce)...)

	aitagKey := aicommon.GetToolParamAITagActionKey("content")
	require.Contains(t, action.GetString(aitagKey), "# 目录结构",
		"AITAG block should be captured into __aitag__content")

	require.Equal(t, sampleTodoContent, action.GetString("content"))

	nested := action.GetParams()
	require.Empty(t, nested.GetString("content"),
		"nested GetParams() still lacks content; fix must merge AITAG at tool invocation")

	merged := MergeLoopActionToolParams(action, action.GetParams(), []string{"content"})
	require.Contains(t, merged.GetString("content"), "# 目录结构")
	require.Contains(t, merged.GetString("content"), "internal/")
	require.NotContains(t, merged.GetString("content"), sampleTodoContent,
		"merged tool params must use AITAG directory tree, not polluted flat content")
	require.Equal(t, nested.GetString("file"), merged.GetString("file"))
	require.True(t, merged.GetBool("force"))
	require.Empty(t, merged.GetString("identifier"))
	require.Empty(t, merged.GetString("human_readable_thought"))
	require.Nil(t, merged["next_movements"])
}

func TestMergeLoopActionToolParams_stripsLoopMetadata(t *testing.T) {
	raw := sampleWriteFileWithNextMovementsAIResponse(sampleWriteFileNonce)
	action := parseSampleWriteFileAction(t, raw, toolParamContentAITagOptions(sampleWriteFileNonce)...)

	merged := MergeLoopActionToolParams(action, action.GetParams(), []string{"content"})
	require.NotContains(t, merged, "identifier")
	require.NotContains(t, merged, "human_readable_thought")
	require.NotContains(t, merged, "next_movements")
	require.NotContains(t, merged, aicommon.GetToolParamAITagActionKey("content"))
	require.Contains(t, merged.GetString("content"), "# 目录结构")
}

func TestBuildLoopActionToolInvokeParams_mergesAITAGForConvertedToolAction(t *testing.T) {
	writeFileTool := aitool.NewWithoutCallback(
		"write_file",
		aitool.WithDescription("write file"),
		aitool.WithStringParam("file", aitool.WithParam_Required(true)),
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithBoolParam("force"),
	)

	raw := sampleWriteFileWithNextMovementsAIResponse(sampleWriteFileNonce)
	action := parseSampleWriteFileAction(t, raw, toolParamContentAITagOptions(sampleWriteFileNonce)...)

	merged := BuildLoopActionToolInvokeParams(action, writeFileTool)
	require.Contains(t, merged.GetString("content"), "# 目录结构")
	require.NotContains(t, merged.GetString("content"), sampleTodoContent)
	require.NotEmpty(t, merged.GetString("file"))
	require.True(t, merged.GetBool("force"))
	require.Empty(t, merged.GetString("identifier"))
	require.Nil(t, merged["next_movements"])

	valid, errs := writeFileTool.ValidateParams(merged)
	require.True(t, valid, "converted tool action params should pass validation: %v", errs)
}
