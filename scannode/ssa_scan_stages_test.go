package scannode

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stage outcomes reported by the scan script must reach artifact metrics so the
// platform can render which stages ran without re-deriving them from jobs.
func TestScanStagesReachArtifactMetrics(t *testing.T) {
	stages := []map[string]any{
		{"stage": "collect", "status": "succeeded"},
		{"stage": "inspect", "status": "succeeded", "rule_count": 1, "risk_count": 2},
		{"stage": "analyze", "status": "failed", "error": "rule set aborted"},
	}
	meta := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{
		"program_name": "project-a",
		"risk_count":   2,
		"stages":       stages,
	}})
	require.NotEmpty(t, meta.ScanStages)

	metrics, err := buildSSAArtifactMetricsPayload(&SSAArtifactReadyEvent{
		ScanStages: meta.ScanStages,
		Metrics:    json.RawMessage(`{"upload_ms":7}`),
	})
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(metrics, &payload))
	require.EqualValues(t, 7, payload["upload_ms"])

	reported, ok := payload["scan_stages"].([]any)
	require.True(t, ok, "scan_stages must survive as a list")
	require.Len(t, reported, 3)
	last := reported[2].(map[string]any)
	require.Equal(t, "analyze", last["stage"])
	require.Equal(t, "failed", last["status"])
	require.Equal(t, "rule set aborted", last["error"])
}

// A script that reports no stages (older engines, compile-only paths) must not
// leak an empty scan_stages key into metrics.
func TestScanStagesAbsentWhenNotReported(t *testing.T) {
	meta := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{"total_lines": 12}})
	require.Empty(t, meta.ScanStages)

	metrics, err := buildSSAArtifactMetricsPayload(&SSAArtifactReadyEvent{})
	require.NoError(t, err)
	require.NotContains(t, string(metrics), "scan_stages")
}
