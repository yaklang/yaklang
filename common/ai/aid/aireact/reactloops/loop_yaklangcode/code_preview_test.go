package loop_yaklangcode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestIsYaklangCodePreviewOnly(t *testing.T) {
	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("preview-test", runtime)
	require.NoError(t, err)

	setYaklangCodePreviewOnly(loop, true)
	require.True(t, isYaklangCodePreviewOnly(loop))

	setYaklangCodePreviewOnly(loop, false)
	loop.Set("editor_file_path", "")
	require.False(t, isYaklangCodePreviewOnly(loop))

	loop.Set("editor_file_path", "/tmp/foo.yak")
	require.False(t, isYaklangCodePreviewOnly(loop))

	loop2, err := reactloops.NewReActLoop("preview-fallback-test", runtime)
	require.NoError(t, err)
	require.True(t, isYaklangCodePreviewOnly(loop2))
}

func TestResolveYaklangCodePreviewOnly(t *testing.T) {
	t.Run("nil attachments", func(t *testing.T) {
		require.True(t, resolveYaklangCodePreviewOnly(nil))
	})
	t.Run("directory_path only", func(t *testing.T) {
		ctx := &YaklangEditorContext{WorkspacePath: "/tmp/workspace"}
		require.True(t, resolveYaklangCodePreviewOnly(ctx))
	})
	t.Run("file_path attached", func(t *testing.T) {
		ctx := &YaklangEditorContext{EditorFile: "/tmp/demo.yak"}
		require.False(t, resolveYaklangCodePreviewOnly(ctx))
	})
	t.Run("workspace and file_path", func(t *testing.T) {
		ctx := &YaklangEditorContext{
			WorkspacePath: "/tmp/workspace",
			EditorFile:    "/tmp/workspace/demo.yak",
		}
		require.False(t, resolveYaklangCodePreviewOnly(ctx))
	})
}

func TestNewYaklangPreviewCodePath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	path, err := newYaklangPreviewCodePath()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(filepath.Clean(path), filepath.Clean(filepath.Join(base, "code"))))
	require.Contains(t, filepath.Base(path), "gen_code_")
	require.Contains(t, path, ".yak")
}

func TestPersistYaklangPreviewCode(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("preview-persist-test", runtime)
	require.NoError(t, err)

	path, err := newYaklangPreviewCodePath()
	require.NoError(t, err)
	loop.Set("filename", path)

	written, err := persistYaklangPreviewCode(loop, "println(\"preview\")")
	require.NoError(t, err)
	require.Equal(t, path, written)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "println(\"preview\")", string(data))
}
