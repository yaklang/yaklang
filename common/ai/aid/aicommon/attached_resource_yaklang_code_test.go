package aicommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYaklangEditorContextFromAttachedFull(t *testing.T) {
	payload := `{"path":"/tmp/project/foo.yak","startLine":3,"endLine":5,"language":"yak","content":"println(1)"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, YaklangAttachedResourceKeyWorkspaceDirectory, "/tmp/project"),
		NewAttachedResource(AttachedResourceTypeFile, YaklangAttachedResourceKeyEditorFile, "/tmp/project/foo.yak"),
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean("/tmp/project"), ctx.WorkspacePath)
	require.Equal(t, filepath.Clean("/tmp/project/foo.yak"), ctx.EditorFile)
	require.NotNil(t, ctx.Selection)
	require.Equal(t, 3, ctx.Selection.StartLine)
}

func TestFormatYaklangEditorContextMarkdown(t *testing.T) {
	out := FormatYaklangEditorContextMarkdown(&YaklangEditorContext{
		WorkspacePath: "/tmp/project",
		EditorFile:    "/tmp/project/foo.yak",
	})
	require.Contains(t, out, "Workspace: `/tmp/project`")
	require.Contains(t, out, "Open File: `/tmp/project/foo.yak`")
}

func TestInferYaklangEditorFileFromUserInput(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "html", "assets")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	yakPath := filepath.Join(nested, "123.yak")
	require.NoError(t, os.WriteFile(yakPath, []byte("// demo"), 0o644))

	got := InferYaklangEditorFileFromUserInput("在当前打开的123.yak文件里生成端口扫描代码", root)
	require.Equal(t, filepath.Clean(yakPath), filepath.Clean(got))

	got = InferYaklangEditorFileFromUserInput(":codeBlockTag[123.yak (1-4)] 修正参数", root)
	require.Equal(t, filepath.Clean(yakPath), filepath.Clean(got))

	got = InferYaklangEditorFileFromUserInput("写一个 hello yak 脚本", root)
	require.Empty(t, got)
}

func TestEnrichYaklangEditorContextFromUserInput(t *testing.T) {
	root := t.TempDir()
	yakPath := filepath.Join(root, "123.yak")
	require.NoError(t, os.WriteFile(yakPath, nil, 0o644))

	ctx := &YaklangEditorContext{WorkspacePath: root}
	EnrichYaklangEditorContextFromUserInput(ctx, "请在123.yak里生成代码")
	require.Equal(t, filepath.Clean(yakPath), filepath.Clean(ctx.EditorFile))
	require.False(t, ctx.IsCreateMode())
}

func TestYaklangEditorContextIsCreateMode(t *testing.T) {
	require.True(t, (*YaklangEditorContext)(nil).IsCreateMode())
	require.True(t, (&YaklangEditorContext{WorkspacePath: "/tmp/workspace"}).IsCreateMode())
	require.False(t, (&YaklangEditorContext{EditorFile: "/tmp/demo.yak"}).IsCreateMode())
}

func TestYaklangAttachedInitialCode(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":1,"endLine":2,"language":"yak","content":"println(1)"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, ok := YaklangAttachedInitialCode(ctx)
	require.True(t, ok)
	require.Equal(t, "println(1)", code)

	_, ok = YaklangAttachedInitialCode(&YaklangEditorContext{EditorFile: "/tmp/foo.yak"})
	require.False(t, ok)
}

func TestResolveYaklangInitTargetPath(t *testing.T) {
	ctx := &YaklangEditorContext{EditorFile: "/tmp/attached.yak"}
	path, fromAttached := ResolveYaklangInitTargetPath(ctx, "/tmp/liteforge.yak")
	require.True(t, fromAttached)
	require.Equal(t, "/tmp/attached.yak", path)

	path, fromAttached = ResolveYaklangInitTargetPath(nil, "/tmp/liteforge.yak")
	require.False(t, fromAttached)
	require.Equal(t, "/tmp/liteforge.yak", path)
}

func TestResolveYaklangInitFullCodePrefersAttachedSelection(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":1,"endLine":2,"language":"yak","content":"attached content"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, fromAttached := ResolveYaklangInitFullCode(ctx, "disk content")
	require.True(t, fromAttached)
	require.Equal(t, "attached content", code)

	code, fromAttached = ResolveYaklangInitFullCode(nil, "disk content")
	require.False(t, fromAttached)
	require.Equal(t, "disk content", code)
}
