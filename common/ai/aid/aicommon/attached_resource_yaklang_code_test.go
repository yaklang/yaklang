package aicommon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYaklangEditorContextFromAttachedFull(t *testing.T) {
	workspace := filepath.Join("testdata", "project")
	yakPath := filepath.Join(workspace, "foo.yak")
	payload := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":3,"endLine":5,"language":"yak","content":"println(1)"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_DIRECTORY_PATH, workspace),
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean(workspace), ctx.WorkspacePath)
	require.Equal(t, filepath.Clean(yakPath), ctx.EditorFile)
	require.NotNil(t, ctx.Selection)
	require.Equal(t, 3, ctx.Selection.StartLine)
}

func TestFormatYaklangEditorContextMarkdown(t *testing.T) {
	workspace := filepath.Join("testdata", "project")
	yakPath := filepath.Join(workspace, "foo.yak")
	out := FormatYaklangEditorContextMarkdown(&YaklangEditorContext{
		WorkspacePath: workspace,
		EditorFile:    yakPath,
	})
	require.Contains(t, out, "Workspace: `"+workspace+"`")
	require.Contains(t, out, "Open File (writable Type=code): `"+yakPath+"`")
	require.Contains(t, out, "Type=file attachments are reference-only")
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
	require.True(t, (&YaklangEditorContext{WorkspacePath: filepath.Join("testdata", "workspace")}).IsCreateMode())
	require.False(t, (&YaklangEditorContext{EditorFile: filepath.Join("testdata", "demo.yak")}).IsCreateMode())
}

func TestYaklangAttachedInitialCode(t *testing.T) {
	yakPath := filepath.Join("testdata", "foo.yak")
	payload := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":1,"endLine":2,"language":"yak","content":"println(1)"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, ok := YaklangAttachedInitialCode(ctx)
	require.True(t, ok)
	require.Equal(t, "println(1)", code)

	_, ok = YaklangAttachedInitialCode(&YaklangEditorContext{EditorFile: yakPath})
	require.False(t, ok)
}

func TestResolveYaklangInitTargetPath(t *testing.T) {
	attached := filepath.Join("testdata", "attached.yak")
	liteforge := filepath.Join("testdata", "liteforge.yak")
	ctx := &YaklangEditorContext{EditorFile: attached}
	path, fromAttached := ResolveYaklangInitTargetPath(ctx, liteforge)
	require.True(t, fromAttached)
	require.Equal(t, filepath.Clean(attached), path)

	path, fromAttached = ResolveYaklangInitTargetPath(nil, liteforge)
	require.False(t, fromAttached)
	require.Equal(t, filepath.Clean(liteforge), path)
}

func TestResolveYaklangInitFullCodePrefersDiskWhenEditorFile(t *testing.T) {
	yakPath := filepath.Join("testdata", "foo.yak")
	payload := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":1,"endLine":2,"language":"yak","content":"attached content"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, fromSelection := ResolveYaklangInitFullCode(ctx, "disk content")
	require.False(t, fromSelection)
	require.Equal(t, "disk content", code)
}

func TestResolveYaklangInitFullCodeUsesAttachedWhenDiskEmpty(t *testing.T) {
	yakPath := filepath.Join("testdata", "foo.yak")
	payload := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":28,"endLine":31,"language":"yak","content":"println(1)"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, fromSelection := ResolveYaklangInitFullCode(ctx, "")
	require.True(t, fromSelection)
	require.Equal(t, "println(1)", code)
}

func TestYaklangCodeLineBase(t *testing.T) {
	ctx := &YaklangEditorContext{
		Selection: &AttachedCodeSelection{StartLine: 28, EndLine: 31, Content: "x"},
	}
	require.Equal(t, 27, YaklangCodeLineBase(ctx, true))
	require.Equal(t, 0, YaklangCodeLineBase(ctx, false))
	require.Equal(t, 0, YaklangCodeLineBase(nil, true))
}

func TestResolveYaklangInitFullCodeSelectionOnlyCreateMode(t *testing.T) {
	payload := `{"startLine":1,"endLine":2,"language":"yak","content":"attached content"}`
	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload),
	})
	code, fromSelection := ResolveYaklangInitFullCode(ctx, "disk content")
	require.True(t, fromSelection)
	require.Equal(t, "attached content", code)
}

func TestParseYaklangEditorContext_CodeTypeSeparatesFromFileReference(t *testing.T) {
	workspace := filepath.Join("testdata", "project")
	yakPath := filepath.Join(workspace, "iotdb_poc.yak")
	mdPath := filepath.Join("testdata", "security_report.md")
	refYak := filepath.Join(workspace, "helper.yak")

	ctx := ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_DIRECTORY_PATH, workspace),
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, refYak),
	})
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean(yakPath), ctx.EditorFile)
	require.False(t, ctx.IsCreateMode())

	// Pure Type=file reference (.md) must not become delivery target.
	ctx = ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
	})
	if ctx != nil {
		require.False(t, ctx.HasEditorFile())
		require.True(t, ctx.IsCreateMode())
	}

	// Legacy: Type=file .yak still accepted when Type=code is absent.
	ctx = ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_DIRECTORY_PATH, workspace),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
	})
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean(yakPath), ctx.EditorFile)

	filtered := FilterAttachedResourcesExcludeYaklangDelivery([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_DIRECTORY_PATH, workspace),
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
	})
	require.Len(t, filtered, 2)
	for _, item := range filtered {
		require.False(t, item.HasType(AttachedResourceTypeCode))
	}
}

func TestAttachedCodeResourceData_NoTimelineDump(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource(
		AttachedResourceTypeCode,
		CONTEXT_PROVIDER_KEY_FILE_PATH,
		filepath.Join("testdata", "demo.yak"),
	))
	require.NoError(t, err)
	require.Equal(t, AttachedResourceTypeCode, resource.Type())
	require.Empty(t, resource.ToAttachData(nil))
}

func TestIsYaklangScriptDeliveryPath(t *testing.T) {
	require.True(t, IsYaklangScriptDeliveryPath(filepath.Join("testdata", "demo.yak")))
	require.True(t, IsYaklangScriptDeliveryPath(filepath.Join("testdata", "a.YAK")))
	require.False(t, IsYaklangScriptDeliveryPath(filepath.Join("testdata", "security_report.md")))
	require.False(t, IsYaklangScriptDeliveryPath(``))
}

func TestExtractMentionPathsFromUserInput(t *testing.T) {
	input := `:mention[C:\Users\13766\Downloads\02\_security\_report.md]{mentionId="C:\Users\13766\Downloads\02_security_report.md"} 根据这个报告编写yak脚本`
	paths := ExtractMentionPathsFromUserInput(input)
	require.NotEmpty(t, paths)
	found := false
	for _, p := range paths {
		if strings.Contains(strings.ToLower(filepath.Base(p)), "security") {
			found = true
			break
		}
	}
	require.True(t, found, "expected mention path, got %#v", paths)
}

func TestParseYaklangEditorContext_IgnoresMentionMarkdownPrefersYak(t *testing.T) {
	workspace := filepath.FromSlash("/tmp/project")
	yakPath := filepath.Join(workspace, "iotdb_poc.yak")
	mdPath := filepath.FromSlash(`/Users/me/Downloads/02_security_report.md`)
	userInput := `:mention[` + mdPath + `]{mentionId="` + mdPath + `"} 根据这个报告编写yak脚本`

	ctx := ParseYaklangEditorContextFromAttachedWithUserInput([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_DIRECTORY_PATH, workspace),
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
		NewAttachedResource(AttachedResourceTypeCode, CONTEXT_PROVIDER_KEY_FILE_PATH, yakPath),
	}, userInput)
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Clean(yakPath), ctx.EditorFile)
	require.False(t, ctx.IsCreateMode())
}

func TestParseYaklangEditorContext_MentionOnlyIsCreateMode(t *testing.T) {
	mdPath := filepath.FromSlash(`C:\Users\13766\Downloads\02_security_report.md`)
	userInput := `:mention[` + mdPath + `]{mentionId="` + mdPath + `"} 根据这个报告编写yak脚本`
	ctx := ParseYaklangEditorContextFromAttachedWithUserInput([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
	}, userInput)
	// Workspace-less mention-only: may be nil or create-mode without EditorFile
	if ctx != nil {
		require.True(t, ctx.IsCreateMode())
		require.False(t, ctx.HasEditorFile())
	}

	// Non-.yak file_path alone (no FreeInput) still must not become EditorFile.
	ctx = ParseYaklangEditorContextFromAttached([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeFile, CONTEXT_PROVIDER_KEY_FILE_PATH, mdPath),
	})
	if ctx != nil {
		require.False(t, ctx.HasEditorFile())
		require.True(t, ctx.IsCreateMode())
	}
}

func TestResolveYaklangInitTargetPath_RejectsNonYak(t *testing.T) {
	ctx := &YaklangEditorContext{EditorFile: `/tmp/report.md`}
	path, fromAttached := ResolveYaklangInitTargetPath(ctx, `/tmp/liteforge.yak`)
	require.False(t, fromAttached)
	require.Equal(t, filepath.Clean(`/tmp/liteforge.yak`), path)

	path, fromAttached = ResolveYaklangInitTargetPath(ctx, `/tmp/report.md`)
	require.False(t, fromAttached)
	require.Empty(t, path)
}

func TestEnrichYaklangEditorContextFromUserInput_ClearsNonYak(t *testing.T) {
	root := t.TempDir()
	yakPath := filepath.Join(root, "demo.yak")
	require.NoError(t, os.WriteFile(yakPath, nil, 0o644))

	ctx := &YaklangEditorContext{
		WorkspacePath: root,
		EditorFile:    filepath.Join(root, "report.md"),
	}
	EnrichYaklangEditorContextFromUserInput(ctx, "请在demo.yak里生成代码")
	require.Equal(t, filepath.Clean(yakPath), filepath.Clean(ctx.EditorFile))
}
