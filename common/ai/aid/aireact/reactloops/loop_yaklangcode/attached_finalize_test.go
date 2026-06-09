package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestAttachedInitialCode(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":1,"endLine":2,"language":"yak","content":"println(1)"}`
	ctx := yaklangEditorContextFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, payload),
	})
	code, ok := attachedInitialCode(ctx)
	require.True(t, ok)
	require.Equal(t, "println(1)", code)

	_, ok = attachedInitialCode(&YaklangEditorContext{EditorFile: "/tmp/foo.yak"})
	require.False(t, ok)
}

func TestResolveYaklangInitTargetPath(t *testing.T) {
	ctx := &YaklangEditorContext{EditorFile: "/tmp/attached.yak"}
	path, fromAttached := resolveYaklangInitTargetPath(ctx, "/tmp/liteforge.yak")
	require.True(t, fromAttached)
	require.Equal(t, "/tmp/attached.yak", path)

	path, fromAttached = resolveYaklangInitTargetPath(nil, "/tmp/liteforge.yak")
	require.False(t, fromAttached)
	require.Equal(t, "/tmp/liteforge.yak", path)
}

func TestResolveYaklangInitFullCodePrefersAttachedSelection(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":1,"endLine":2,"language":"yak","content":"attached content"}`
	ctx := yaklangEditorContextFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, payload),
	})
	code, fromAttached := resolveYaklangInitFullCode(ctx, "disk content")
	require.True(t, fromAttached)
	require.Equal(t, "attached content", code)

	code, fromAttached = resolveYaklangInitFullCode(nil, "disk content")
	require.False(t, fromAttached)
	require.Equal(t, "disk content", code)
}

func TestBuildYaklangAnalyzeRequirementPromptWithAttachedPath(t *testing.T) {
	out := buildYaklangAnalyzeRequirementPrompt(yaklangAnalyzeRequirementOptions{
		userInput:       "fix scan timeout",
		hasAttachedPath: true,
		attachedPath:    "/tmp/project/scan.yak",
		workspacePath:   "/tmp/project",
		hasGrepSearcher: true,
	})
	require.Contains(t, out, "已知编辑器上下文")
	require.Contains(t, out, "/tmp/project/scan.yak")
	require.NotContains(t, out, "判断文件操作类型")
}

func TestBuildYaklangAnalyzeRequirementPromptWithoutAttachedPath(t *testing.T) {
	out := buildYaklangAnalyzeRequirementPrompt(yaklangAnalyzeRequirementOptions{
		userInput:       "write port scan",
		hasAttachedPath: false,
		hasGrepSearcher: true,
	})
	require.Contains(t, out, "判断文件操作类型")
}

func TestBuildYaklangAnalyzeRequirementToolOptionsSkipsFileDetectWhenAttached(t *testing.T) {
	opts := yaklangAnalyzeRequirementOptions{
		hasAttachedPath: true,
		hasGrepSearcher: true,
	}
	attachedOpts := buildYaklangAnalyzeRequirementToolOptions(opts, true)
	require.NotEmpty(t, attachedOpts)

	opts.hasAttachedPath = false
	defaultOpts := buildYaklangAnalyzeRequirementToolOptions(opts, true)
	require.Greater(t, len(defaultOpts), len(attachedOpts))
}
