package loop_yaklangcode

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestYaklangEditorContextFromAttachedFull(t *testing.T) {
	payload := `{"path":"/tmp/project/foo.yak","startLine":3,"endLine":5,"language":"yak","content":"println(1)"}`
	ctx := yaklangEditorContextFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyWorkspaceDirectory, "/tmp/project"),
		aicommon.NewAttachedResource(AttachedResourceTypeFile, AttachedResourceKeyEditorFile, "/tmp/project/foo.yak"),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, payload),
	})
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean("/tmp/project"), ctx.WorkspacePath)
	require.Equal(t, filepath.Clean("/tmp/project/foo.yak"), ctx.EditorFile)
	require.NotNil(t, ctx.Selection)
	require.Equal(t, 3, ctx.Selection.StartLine)
}

func TestFormatYaklangEditorContextMarkdown(t *testing.T) {
	out := formatYaklangEditorContextMarkdown(&YaklangEditorContext{
		WorkspacePath: "/tmp/project",
		EditorFile:    "/tmp/project/foo.yak",
	})
	require.Contains(t, out, "Workspace: `/tmp/project`")
	require.Contains(t, out, "Open File: `/tmp/project/foo.yak`")
}
