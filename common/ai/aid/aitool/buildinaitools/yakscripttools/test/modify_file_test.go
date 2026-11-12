package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
)

func getModifyFileTool(t *testing.T) *aitool.Tool {
	t.Helper()
	content, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/fs/modify_file.yak")
	require.NoError(t, err)
	meta := yakscripttools.LoadYakScriptToAiTools("modify_file", string(content))
	require.NotNil(t, meta)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{meta})
	require.Len(t, tools, 1)
	return tools[0]
}

func invokeModifyFile(t *testing.T, params aitool.InvokeParams) (*aitool.ToolResult, *aitool.ToolExecutionResult) {
	t.Helper()
	result, err := getModifyFileTool(t).InvokeWithParams(params)
	require.NoError(t, err)
	require.NotNil(t, result)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	return result, execution
}

func modifyFileText(execution *aitool.ToolExecutionResult) string {
	if execution == nil {
		return ""
	}
	return utils.InterfaceToString(execution.Result) + "\n" + execution.CombinedOutput
}

func writeModifyFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestModifyFileRejectsOutOfRangeLineWithoutAppending(t *testing.T) {
	const original = "one\ntwo\nthree\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"line-number": 9999,
		"mode":        "replace",
		"content":     "unexpected append",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusFailed, status)
	require.Contains(t, modifyFileText(execution), "mode=append explicitly")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(actual))
}

func TestModifyFileAppendIsExplicit(t *testing.T) {
	path := writeModifyFixture(t, "one\ntwo")

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":    path,
		"mode":    "append",
		"content": "three\nfour",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status)

	output := modifyFileText(execution)
	require.Contains(t, output, "DIFF:")
	require.Contains(t, output, "VERIFIED CONTENT:")
	require.Contains(t, output, "+three")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\ntwo\nthree\nfour", string(actual))
}

func TestModifyFileRangeReplaceIsSingleSafeOperation(t *testing.T) {
	path := writeModifyFixture(t, "one\ntwo\nthree\nfour\n")

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"line-number": 2,
		"end-line":    3,
		"mode":        "replace-range",
		"content":     "TWO\nTHREE",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status)
	require.Contains(t, modifyFileText(execution), "replaced line range 2-3")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "one\nTWO\nTHREE\nfour\n", string(actual))
}

func TestModifyFileContextPatchRequiresUniqueMatch(t *testing.T) {
	path := writeModifyFixture(t, "before\ncall old();\nafter\n")

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"mode":        "patch",
		"old-content": "call old();",
		"content":     "call new();",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status)
	require.Contains(t, modifyFileText(execution), "replaced unique context match")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "before\ncall new();\nafter\n", string(actual))
}

func TestModifyFileContextPatchPreservesUTF8BeforeMatch(t *testing.T) {
	const original = "// 中文注释：热搜榜交互\n" +
		"// emoji: 🔥🚀\n" +
		"const title = '旧标题';\n" +
		"console.log(title);\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"mode":        "patch",
		"old-content": "const title = '旧标题';",
		"content":     "const title = '新标题';",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status, modifyFileText(execution))
	require.Contains(t, modifyFileText(execution), "replaced unique context match")
	require.Contains(t, modifyFileText(execution), "  3 | const title = '新标题';")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(original, "旧标题", "新标题", 1), string(actual))
}

func TestModifyFileContextPatchOccurrencePreservesUTF8(t *testing.T) {
	const original = "// 中文前缀\ncall old();\n// 中间 🚀\ncall old();\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"mode":        "patch",
		"old-content": "call old();",
		"content":     "call new();",
		"occurrence":  2,
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status, modifyFileText(execution))
	require.Contains(t, modifyFileText(execution), "  4 | call new();")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "// 中文前缀\ncall old();\n// 中间 🚀\ncall new();\n", string(actual))
}

func TestModifyFileContextPatchPreservesCRLF(t *testing.T) {
	const original = "// 中文\r\nold();\r\ntail();\r\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"mode":        "patch",
		"old-content": "old();",
		"content":     "new();",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status, modifyFileText(execution))

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "// 中文\r\nnew();\r\ntail();\r\n", string(actual))
}

func TestModifyFileContextPatchRejectsAmbiguousMatch(t *testing.T) {
	const original = "x();\nkeep\nx();\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":        path,
		"mode":        "patch",
		"old-content": "x();",
		"content":     "y();",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusFailed, status)
	require.Contains(t, modifyFileText(execution), "matched 2 times")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(actual))
}

func TestModifyFileRejectsStaleExpectedHash(t *testing.T) {
	const original = "current\n"
	path := writeModifyFixture(t, original)

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":            path,
		"mode":            "append",
		"content":         "stale write",
		"expected-sha256": strings.Repeat("0", 64),
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusFailed, status)
	require.Contains(t, modifyFileText(execution), "expected-sha256")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(actual))
}

func TestModifyFileEmptyFileCanBeAppended(t *testing.T) {
	path := writeModifyFixture(t, "")

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":    path,
		"mode":    "append",
		"content": "first line\n",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status)
	require.Contains(t, modifyFileText(execution), "+first line")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "first line\n", string(actual))
}

func TestModifyFileAppendAfterUTF8ReportsCorrectLine(t *testing.T) {
	path := writeModifyFixture(t, "第一行\n第二行")

	result, execution := invokeModifyFile(t, aitool.InvokeParams{
		"file":    path,
		"mode":    "append",
		"content": "第三行",
	})
	status, _ := result.GetExecutionStatus()
	require.Equal(t, aitool.ToolExecutionStatusSucceeded, status, modifyFileText(execution))
	require.Contains(t, modifyFileText(execution), "  3 | 第三行")

	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "第一行\n第二行\n第三行", string(actual))
}
