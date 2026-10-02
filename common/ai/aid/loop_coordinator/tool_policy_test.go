package loop_coordinator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCoordinatorLoopToolPolicy(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"read_file", "list_dir", "grep"} {
		require.True(t, AllowedTool(name, nil, dir))
	}
	for _, name := range []string{"exec", "exec_command", "yak_script", "write_yaklang_code", "unknown_mcp", "remove_file"} {
		require.False(t, AllowedTool(name, nil, dir))
	}
	for _, path := range []string{"artifacts/report.md", filepath.Join(dir, "artifacts", "nested", "report.md")} {
		require.True(t, AllowedTool("write_file", aitool.InvokeParams{"file": path}, dir))
	}
	for _, path := range []string{"README.md", "artifacts/../../outside.md", "artifacts/run.yak", "artifacts"} {
		require.False(t, AllowedTool("write_file", aitool.InvokeParams{"file": path}, dir))
	}
}

func TestCoordinatorLoopToolPolicyRejectsSymlinkEscape(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "artifacts")); err != nil {
		t.Skipf("host cannot create symlinks: %v", err)
	}
	require.False(t, AllowedTool("write_file", aitool.InvokeParams{"file": "artifacts/report.md"}, dir))
}
