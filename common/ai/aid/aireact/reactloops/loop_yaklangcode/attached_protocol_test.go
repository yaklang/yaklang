package loop_yaklangcode

// YakRunner / AIInputEvent AttachedResourceInfo protocol tests (unit level).
//
// Run:
//   go test -v -run TestYakRunnerProtocol_ ./common/ai/aid/aireact/reactloops/loop_yaklangcode/...

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// TestYakRunnerProtocol_1_WorkspaceAndFilePathAttachments verifies the「选择工作区」协议：
//
//	{ Type: "file", Key: "directory_path", Value: workspace }
//	{ Type: "file", Key: "file_path", Value: open file }
func TestYakRunnerProtocol_1_WorkspaceAndFilePathAttachments(t *testing.T) {
	workspace := filepath.FromSlash("/Users/me/yakit-projects/demo")
	editorFile := filepath.Join(workspace, "scan.yak")

	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyWorkspaceDirectory, workspace),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyEditorFile, editorFile),
	}

	ctx := yaklangEditorContextFromAttached(attached)
	require.NotNil(t, ctx)
	assert.Equal(t, filepath.Clean(workspace), ctx.WorkspacePath)
	assert.Equal(t, filepath.Clean(editorFile), ctx.EditorFile)
	assert.False(t, ctx.HasSelection())

	md := formatYaklangEditorContextMarkdown(ctx)
	assert.Contains(t, md, "Workspace:")
	assert.Contains(t, md, "yakit-projects")
	assert.Contains(t, md, "scan.yak")

	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("protocol-1", runtime)
	require.NoError(t, err)

	editorCtx := initYaklangEditorContextFromAttached(runtime, loop, attached)
	require.NotNil(t, editorCtx)
	assert.Equal(t, filepath.Clean(workspace), loop.Get("workspace_path"))
	assert.Equal(t, filepath.Clean(editorFile), loop.Get("editor_file_path"))
}

// TestYakRunnerProtocol_2_SelectedContentAttachment verifies the「选择代码片段」协议：
//
//	{ Type: "selected", Key: "content", Value: AttachedCodeSelection JSON }
func TestYakRunnerProtocol_2_SelectedContentAttachment(t *testing.T) {
	workspace := filepath.FromSlash("/Users/me/yakit-projects/demo")
	editorFile := filepath.Join(workspace, "scan.yak")
	selectionJSON := `{"path":"/Users/me/yakit-projects/demo/scan.yak","startLine":10,"endLine":18,"language":"yak","content":"println(\"hi\")"}`

	attached := []*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyWorkspaceDirectory, workspace),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyEditorFile, editorFile),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, selectionJSON),
	}

	ctx := yaklangEditorContextFromAttached(attached)
	require.NotNil(t, ctx)
	require.NotNil(t, ctx.Selection)
	assert.Equal(t, 10, ctx.Selection.StartLine)
	assert.Equal(t, 18, ctx.Selection.EndLine)
	assert.Equal(t, "yak", ctx.Selection.Language)
	assert.Equal(t, `println("hi")`, ctx.Selection.Content)
	assert.Equal(t, filepath.Clean(editorFile), ctx.EditorFile)

	code, ok := attachedInitialCode(ctx)
	require.True(t, ok)
	assert.Equal(t, `println("hi")`, code)

	md := formatYaklangEditorContextMarkdown(ctx)
	assert.Contains(t, md, "Selection Lines: 10-18")
	assert.Contains(t, md, "Selection Language: yak")
}
