package yakit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCleanupAISpaceWorkDirsForSessions_RemovesReferencedDirs(t *testing.T) {
	projectDB, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, projectDB.AutoMigrate(&schema.AIAgentRuntime{}).Error)

	sessionID := "sess-" + uuid.NewString()
	workDir := filepath.Join(consts.GetDefaultAISpaceDir(), "test-delete-"+uuid.NewString())
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "artifact.txt"), []byte("x"), 0o644))
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	require.NoError(t, projectDB.Create(&schema.AIAgentRuntime{
		Uuid:              uuid.NewString(),
		PersistentSession: sessionID,
		WorkDir:           workDir,
	}).Error)

	removed, err := CleanupAISpaceWorkDirsForSessions(projectDB, []string{sessionID})
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	_, statErr := os.Stat(workDir)
	require.True(t, os.IsNotExist(statErr))
}

func TestRemoveAISpaceWorkDirs_SkipsUnsafePaths(t *testing.T) {
	unsafeDir := filepath.Join(os.TempDir(), "unsafe-aispace-"+uuid.NewString())
	require.NoError(t, os.MkdirAll(unsafeDir, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(unsafeDir) })

	removed := RemoveAISpaceWorkDirs([]string{unsafeDir, "/etc/passwd"})
	require.Equal(t, 0, removed)
	_, err := os.Stat(unsafeDir)
	require.NoError(t, err)
}

func TestRemoveAISpaceWorkDirs_DedupesPaths(t *testing.T) {
	workDir := filepath.Join(consts.GetDefaultAISpaceDir(), "test-dedupe-"+uuid.NewString())
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })

	removed := RemoveAISpaceWorkDirs([]string{workDir, workDir})
	require.Equal(t, 1, removed)
	_, err := os.Stat(workDir)
	require.True(t, os.IsNotExist(err))
}
