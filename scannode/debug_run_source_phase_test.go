package scannode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebugRunAnalyzesSourceScanPhase(t *testing.T) {
	analysis := &DebugRunAnalysis{Status: "completed"}
	analysis.setPhasesFromLog(`[INFO] 2026-08-30 19:00:00 [SOURCE-SCAN] 项目: core, 语言: java
[INFO] 2026-08-30 19:00:01 [SOURCE-SCAN] 开始 source-mode 规则扫描 (mode=source)
[INFO] 2026-08-30 19:00:03 [SOURCE-SCAN] 完成，发现 2 个风险
`)

	require.Len(t, analysis.Phases, 1)
	require.Equal(t, "source-scan", analysis.Phases[0].Phase)
	require.Equal(t, "completed", analysis.Phases[0].Status)
	require.NotNil(t, analysis.Phases[0].StartedAt)
	require.NotNil(t, analysis.Phases[0].FinishedAt)
}

func TestDebugRunKeepsSourceScanRunningWithoutCompletion(t *testing.T) {
	analysis := &DebugRunAnalysis{Status: "running"}
	analysis.setPhasesFromLog(`[INFO] 2026-08-30 19:00:00 [SOURCE-SCAN] 开始 source-mode 规则扫描
`)

	require.Len(t, analysis.Phases, 1)
	require.Equal(t, "source-scan", analysis.Phases[0].Phase)
	require.Equal(t, "running", analysis.Phases[0].Status)
	require.Nil(t, analysis.Phases[0].FinishedAt)
}
