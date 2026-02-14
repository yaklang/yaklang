package aicommon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockArtifactsConfig implements AICallerConfigIf.GetOrCreateWorkDir for testing
type mockArtifactsConfig struct {
	workDir string
}

func (m *mockArtifactsConfig) GetOrCreateWorkDir() string {
	return m.workDir
}

func TestArtifactsContextProvider_BasicOutput(t *testing.T) {
	// Setup: create a temp directory with mock task artifact files
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create task directories and files
	task1Dir := filepath.Join(tmpDir, "task_1-1_network_scan")
	require.NoError(t, os.MkdirAll(filepath.Join(task1Dir, "tool_calls"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(task1Dir, "tool_calls", "1_bash_scan.md"), []byte("# Tool Call: bash\nscan result"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(task1Dir, "task_1_1_result_summary.txt"), []byte("scan completed successfully"), 0644))

	task2Dir := filepath.Join(tmpDir, "task_1-2_security_analysis")
	require.NoError(t, os.MkdirAll(task2Dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(task2Dir, "task_1_2_result_summary.txt"), []byte("no vulnerabilities found"), 0644))

	// Create a real Config to use as the AICallerConfigIf (it has GetOrCreateWorkDir)
	cfg := NewConfig(context.Background())
	cfg.Workdir = tmpDir

	// Call the provider
	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)
	require.NotEmpty(t, result, "artifacts context should not be empty when files exist")

	t.Logf("artifacts context output:\n%s", result)

	// Verify the output contains key information
	assert.Contains(t, result, "# Session Artifacts")
	assert.Contains(t, result, tmpDir, "should contain the artifacts directory path")
	assert.Contains(t, result, "task_1-1_network_scan", "should contain task directory name")
	assert.Contains(t, result, "task_1-2_security_analysis", "should contain second task directory name")
	assert.Contains(t, result, "1_bash_scan.md", "should contain tool call filename")
	assert.Contains(t, result, "task_1_1_result_summary.txt", "should contain result summary filename")
	// File sizes are displayed as B, KB, or MB depending on size
	assert.True(t, strings.Contains(result, "B") || strings.Contains(result, "KB") || strings.Contains(result, "MB"),
		"should contain file size in B/KB/MB format")
}

func TestArtifactsContextProvider_EmptyDir(t *testing.T) {
	// Setup: empty temp directory
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_empty_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := NewConfig(context.Background())
	cfg.Workdir = tmpDir

	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)
	assert.Empty(t, result, "artifacts context should be empty for an empty directory")
}

func TestArtifactsContextProvider_NoWorkDir(t *testing.T) {
	cfg := NewConfig(context.Background())
	// Workdir is "" by default, and workDir is also ""
	// GetOrCreateWorkDir will create a fallback dir, so let's test with an explicitly non-existent path
	// by not setting anything and testing the fallback behavior

	// For this test, we need to ensure GetOrCreateWorkDir returns "" or a non-existent dir
	// Since NewConfig always creates a ContextProviderManager and registers the provider,
	// we test the provider function directly with a config that has no workdir
	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)
	// The provider should handle missing workdir gracefully
	// It will either be empty (if GetOrCreateWorkDir creates a new empty dir) or empty string
	// In either case, no error should occur
	t.Logf("result for no workdir: %q", result)
}

func TestArtifactsContextProvider_8KBLimit(t *testing.T) {
	// Setup: create a directory with many files to exceed 8KB output
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_limit_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create many task directories with long names and many files
	for i := 0; i < 50; i++ {
		taskName := strings.Repeat("a", 40) + "_task_dir"
		taskDir := filepath.Join(tmpDir, taskName+"_"+strings.Repeat("b", 10)+"_"+time.Now().Format("150405"))
		require.NoError(t, os.MkdirAll(filepath.Join(taskDir, "tool_calls"), 0755))
		for j := 0; j < 20; j++ {
			fileName := strings.Repeat("c", 30) + "_file.md"
			filePath := filepath.Join(taskDir, "tool_calls", fileName)
			require.NoError(t, os.WriteFile(filePath, []byte(strings.Repeat("x", 100)), 0644))
		}
	}

	cfg := NewConfig(context.Background())
	cfg.Workdir = tmpDir

	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)

	// Verify the output is within the 8KB limit
	assert.LessOrEqual(t, len(result), ArtifactsContextMaxBytes,
		"artifacts context should not exceed %d bytes, got %d bytes", ArtifactsContextMaxBytes, len(result))
	t.Logf("artifacts context size: %d bytes (limit: %d)", len(result), ArtifactsContextMaxBytes)
}

func TestArtifactsContextProvider_FileModificationTime(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_mtime_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "task_1-1_test_task")
	require.NoError(t, os.MkdirAll(taskDir, 0755))

	// Create a file and set a known modification time
	filePath := filepath.Join(taskDir, "result.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("test result"), 0644))

	cfg := NewConfig(context.Background())
	cfg.Workdir = tmpDir

	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)

	// Should contain modification time format (HH:MM:SS)
	assert.Contains(t, result, "result.txt")
	assert.Contains(t, result, "modified:")
	t.Logf("output:\n%s", result)
}

func TestArtifactsContextProvider_RootLevelFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_root_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a file directly in the root (not in a task subdirectory)
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "session_summary.txt"), []byte("overall summary"), 0644))

	// Also create a task directory
	taskDir := filepath.Join(tmpDir, "task_1-1_scan")
	require.NoError(t, os.MkdirAll(taskDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "result.txt"), []byte("scan result"), 0644))

	cfg := NewConfig(context.Background())
	cfg.Workdir = tmpDir

	result, err := ArtifactsContextProvider(cfg, nil, "session_artifacts")
	require.NoError(t, err)

	assert.Contains(t, result, "session_summary.txt", "should contain root-level file")
	assert.Contains(t, result, "[root files]", "should have root files section")
	assert.Contains(t, result, "task_1-1_scan", "should contain task directory")
	t.Logf("output:\n%s", result)
}

func TestArtifactsContextProvider_RegisteredInNewConfig(t *testing.T) {
	// Verify that NewConfig automatically registers the session_artifacts provider
	cfg := NewConfig(context.Background())

	// Create a temp dir as workdir and populate it
	tmpDir, err := os.MkdirTemp("", "artifacts_ctx_auto_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	taskDir := filepath.Join(tmpDir, "task_1-1_auto_test")
	require.NoError(t, os.MkdirAll(taskDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "output.txt"), []byte("auto test output"), 0644))

	cfg.Workdir = tmpDir

	// Execute the ContextProviderManager to get DynamicContext
	result := cfg.ContextProviderManager.Execute(cfg, cfg.Emitter)
	assert.Contains(t, result, "Session Artifacts", "DynamicContext should contain session_artifacts provider output")
	assert.Contains(t, result, "task_1-1_auto_test", "DynamicContext should contain task directory name")
	assert.Contains(t, result, "output.txt", "DynamicContext should contain filename")
	t.Logf("DynamicContext output:\n%s", result)
}
