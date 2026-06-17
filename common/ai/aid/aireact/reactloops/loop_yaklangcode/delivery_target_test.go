package loop_yaklangcode

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

func TestResolveYaklangDeliveryTarget(t *testing.T) {
	loop, err := reactloops.NewReActLoop("resolve-target-test", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)

	path, op, err := resolveYaklangDeliveryTarget(loop)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpCreate, op)
	assert.True(t, isYaklangGenCodePath(path))

	genPath := filepath.Join(t.TempDir(), "gen_code_20260616_1451.yak")
	editorPath := filepath.Join(t.TempDir(), "123.yak")
	loop.Set("filename", genPath)
	loop.Set("editor_file_path", editorPath)
	path, op, err = resolveYaklangDeliveryTarget(loop)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(editorPath), path)
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpReplace, op)

	loop.Set("editor_file_path", "")
	path, op, err = resolveYaklangDeliveryTarget(loop)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(genPath), path)
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpCreate, op)
}

func TestResolveYaklangDeliveryTarget_AspaceStagingMapsToGenCode(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	loop, err := reactloops.NewReActLoop("resolve-aspace-test", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)

	loop.Set("filename", filepath.Join(base, "aispace", "yaklang_code_staging_abc.yak"))
	path, op, err := resolveYaklangDeliveryTarget(loop)
	require.NoError(t, err)
	assert.Equal(t, loopinfra.LoopYaklangCodeEventOpCreate, op)
	assert.True(t, isYaklangGenCodePath(path))
	assert.Contains(t, path, filepath.Join(base, "code"))
}

func TestHasYaklangEditorDeliveryTarget(t *testing.T) {
	loop, err := reactloops.NewReActLoop("delivery-target-test", mock.NewMockInvoker(context.Background()))
	require.NoError(t, err)
	require.False(t, hasYaklangEditorDeliveryTarget(loop))

	loop.Set("editor_file_path", "/tmp/foo.yak")
	require.True(t, hasYaklangEditorDeliveryTarget(loop))
}

func TestNewYaklangGenCodePath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YAKIT_HOME", base)

	path, err := newYaklangGenCodePath()
	require.NoError(t, err)
	require.Contains(t, filepath.Clean(path), filepath.Clean(filepath.Join(base, "code")))
	require.Contains(t, filepath.Base(path), "gen_code_")
	require.Contains(t, path, ".yak")
}
