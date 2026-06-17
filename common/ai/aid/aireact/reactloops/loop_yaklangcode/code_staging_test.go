package loop_yaklangcode

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

type stagingTestRuntime struct {
	*mock.MockInvoker
	tmpDir string
}

func (r *stagingTestRuntime) EmitFileArtifactWithExt(name, ext string, data any) string {
	return filepath.Join(r.tmpDir, name+ext)
}

func TestEnsureYaklangLoopStagingFilename_UsesAispacePath(t *testing.T) {
	runtime := &stagingTestRuntime{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		tmpDir:      t.TempDir(),
	}
	loop, err := reactloops.NewReActLoop("staging-test", runtime)
	require.NoError(t, err)

	staging := ensureYaklangLoopStagingFilename(loop, runtime)
	require.NotEmpty(t, staging)
	require.Contains(t, staging, "yaklang_code_staging")
	require.Equal(t, staging, loop.Get("filename"))
	require.Equal(t, staging, loop.Get(yaklangCodeStagingFilenameLoopKey))

	stagingAgain := ensureYaklangLoopStagingFilename(loop, runtime)
	require.Equal(t, staging, stagingAgain)
}
