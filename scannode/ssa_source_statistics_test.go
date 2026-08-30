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
