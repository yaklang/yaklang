package scannode

import (
	"context"
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
		"succeeded":    true,
		"error":        "rule set aborted",
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

	verdict, ok := payload["scan_stages"].(map[string]any)
	require.True(t, ok, "scan_stages must be the wrapped engine verdict")
	require.Equal(t, true, verdict["succeeded"])
	require.Equal(t, "rule set aborted", verdict["error"])
	reported, ok := verdict["stages"].([]any)
	require.True(t, ok, "wrapped stages must survive as a list")
	require.Len(t, reported, 3)
	last := reported[2].(map[string]any)
	require.Equal(t, "analyze", last["stage"])
	require.Equal(t, "failed", last["status"])
	require.Equal(t, "rule set aborted", last["error"])
}

func TestIncompleteStagesReachArtifactMetrics(t *testing.T) {
	meta := parseSSAResultMeta(&ScriptExecutionResult{Data: map[string]any{
		"succeeded":         true,
		"incomplete_stages": true,
		"skipped_stages":    []string{"analyze"},
		"stages": []map[string]any{
			{"stage": "collect", "status": "succeeded"},
			{"stage": "inspect", "status": "succeeded"},
			{"stage": "review", "status": "succeeded"},
		},
	}})
	require.NotEmpty(t, meta.ScanStages)

	var verdict map[string]any
	require.NoError(t, json.Unmarshal(meta.ScanStages, &verdict))
	require.Equal(t, true, verdict["incomplete_stages"])
	skipped, ok := verdict["skipped_stages"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"analyze"}, skipped)
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

// A clone failure has no streamed findings, but the engine still reported
// which stages ran. That verdict must be persisted so diagnostics can show it.
func TestShouldPreserveFailedSSAArtifactWhenOnlyStagesExist(t *testing.T) {
	require.False(t, shouldPreserveFailedSSAArtifact(nil, nil))

	collector := NewSSAArtifactCollectorWithContext(context.Background(), "task", "runtime", "sub")
	t.Cleanup(func() { collector.Cleanup() })
	require.False(t, shouldPreserveFailedSSAArtifact(collector, &ScriptExecutionResult{}))
	require.True(t, shouldPreserveFailedSSAArtifact(collector, &ScriptExecutionResult{
		Data: map[string]any{
			"succeeded": false,
			"error":     "SSA Git clone failed",
			"stages": []map[string]any{
				{"stage": "collect", "status": "failed", "error": "SSA Git clone failed"},
			},
		},
	}))
}
