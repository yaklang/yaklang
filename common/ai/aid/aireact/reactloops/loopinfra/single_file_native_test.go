package loopinfra

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestSingleFileNativeContentIsCallScoped(t *testing.T) {
	runtime := newTestRuntimeForSingleFile(t)
	loop, factory, task := newLoopAndFactory(t, runtime, WithLoopVarsPrefix("report"), WithActionSuffix("section"), WithAITagConfig("GEN_REPORT", "report_content", "report-content", "text/markdown"), WithFileExtension(".md"), WithExitAfterWrite(false))
	defer loop.Release()
	reactloops.WithFunctionCallMode(true)(loop)
	loop.Set("report_content", "STALE TAG BODY MUST NOT BE USED")
	steps := []struct {
		name string
		args map[string]any
		want string
	}{
		{"write_section", map[string]any{"content": "# 报告\n旧内容\n尾行\n"}, "# 报告\n旧内容\n尾行\n"},
		{"modify_section", map[string]any{"content": "新内容 𠮷 \"quoted\"", "old_snippet": "旧内容"}, "# 报告\n新内容 𠮷 \"quoted\"\n尾行\n"},
		{"insert_section", map[string]any{"content": "新增章节\n", "insert_line": 3}, "# 报告\n新内容 𠮷 \"quoted\"\n新增章节\n尾行\n"},
		{"modify_section", map[string]any{"content": "*** Begin Patch\n*** Update File: current\n@@\n-尾行\n+末行\n*** End Patch"}, "# 报告\n新内容 𠮷 \"quoted\"\n新增章节\n末行\n"},
		{"delete_section", map[string]any{"delete_start_line": 3, "delete_end_line": 3}, "# 报告\n新内容 𠮷 \"quoted\"\n末行\n"},
	}
	for _, step := range steps {
		handler, err := loop.GetActionHandler(step.name)
		require.NoError(t, err)
		call := mustBuildAction(t, step.name, step.args)
		require.NoError(t, handler.ActionVerifier(loop, call))
		op := reactloops.NewActionHandlerOperator(task)
		handler.ActionHandler(loop, call, op)
		raw, err := os.ReadFile(loop.Get(factory.GetFilenameVariableName()))
		require.NoError(t, err)
		require.Equal(t, step.want, string(raw))
		require.Equal(t, "STALE TAG BODY MUST NOT BE USED", loop.Get("report_content"))
	}
	handler, err := loop.GetActionHandler("insert_section")
	require.NoError(t, err)
	require.ErrorContains(t, handler.ActionVerifier(loop, mustBuildAction(t, "insert_section", map[string]any{"insert_line": 1})), "arguments.content")
}
