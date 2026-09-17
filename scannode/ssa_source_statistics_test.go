package scannode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceStatisticsMetricsWithZeroRisks(t *testing.T) {
	stats := map[string]any{"schema_version": "ssa-source-statistics.v1", "scope": "compiled_sources", "line_count_kind": "physical", "analyzed_file_count": 3, "analyzed_line_count": 12}
	meta := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{"program_name": "zero-risks", "risk_count": 0, "source_statistics": stats}})
	metrics, err := buildSSAArtifactMetricsPayload(&SSAArtifactReadyEvent{SourceStatistics: meta.SourceStatistics, Metrics: json.RawMessage(`{"upload_ms":7}`)})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(metrics, &payload))
	require.EqualValues(t, 0, payload["risk_count"])
	require.EqualValues(t, 0, payload["file_count"], "risk-file count is independent")
	require.EqualValues(t, 3, payload["source_statistics"].(map[string]any)["analyzed_file_count"])
	require.EqualValues(t, 7, payload["upload_ms"])
	legacy := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{"total_lines": 12}})
	require.Empty(t, legacy.SourceStatistics)
	metrics, err = buildSSAArtifactMetricsPayload(&SSAArtifactReadyEvent{})
	require.NoError(t, err)
	require.NotContains(t, string(metrics), "source_statistics")
}

func TestCompileScaleReachesArtifactMetrics(t *testing.T) {
	meta := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{
		"total_files":      12,
		"handler_files":    8,
		"prehandler_files": 12,
		"total_bytes":      4096,
		"total_lines":      80,
	}})
	require.NotEmpty(t, meta.CompileScale)
	metrics, err := buildSSAArtifactMetricsPayload(&SSAArtifactReadyEvent{CompileScale: meta.CompileScale})
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(metrics, &payload))
	scale := payload["compile_scale"].(map[string]any)
	require.EqualValues(t, 12, scale["total_files"])
	require.EqualValues(t, 4096, scale["total_bytes"])
	require.EqualValues(t, 80, scale["total_lines"])
}
