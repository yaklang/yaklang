package loop_syntaxflow_scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSyntaxflowScanFallbackReport(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "syntaxflow_scan_report_input.md")
	require.NoError(t, os.WriteFile(inPath, []byte("# input\n\npipeline ok\n"), 0o644))

	batchesDir := filepath.Join(dir, "batches")
	require.NoError(t, os.MkdirAll(batchesDir, 0o755))
	batch1 := filepath.Join(batchesDir, "batch_001.md")
	require.NoError(t, os.WriteFile(batch1, []byte("risk A"), 0o644))

	out := generateSyntaxflowScanFallbackReport(inPath, []string{batch1})
	require.Contains(t, out, "扫描输入概要")
	require.Contains(t, out, "pipeline ok")
	require.Contains(t, out, "batch_001.md")
	require.Contains(t, out, "risk A")
	require.True(t, strings.HasPrefix(out, "# SyntaxFlow 扫描报告（自动生成）"))
}
