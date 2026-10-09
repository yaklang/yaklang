package scannode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompanyGenericScriptHasNoSSAArtifactTicketIntent(t *testing.T) {
	params := map[string]interface{}{scannodeCompanyIDParamKey: "company-1", scannodeTaskIDParamKey: "job-1", scannodeAttemptIDParamKey: "attempt-1"}
	require.NoError(t, validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params))
	cfg := scriptSSAArtifactUploadConfig("company-1", params)
	require.Nil(t, cfg)
	collector := NewSSAArtifactCollectorWithContext(context.Background(), "job-1", "attempt-1", "subtask-1")
	t.Cleanup(collector.Cleanup)
	reporter := &ScannerAgentReporter{ssaCollector: collector, ssaUploadCfg: cfg}
	// With no ScanNode there is no ticket transport. A generic empty collector
	// must finalize without even trying to obtain an SSA upload configuration.
	var scanNode *ScanNode
	require.NoError(t, scanNode.finalizeSSAArtifactUpload(context.Background(), reporter, &ScriptExecutionResult{}))
}

func TestCompanySSAZeroRiskStillRequiresArtifactUpload(t *testing.T) {
	params := validCompanyDispatchParams()
	require.NoError(t, validateCompanyDispatchParams("company-1", "job-1", "attempt-1", params))
	cfg := scriptSSAArtifactUploadConfig("company-1", params)
	require.NotNil(t, cfg)
	require.Empty(t, cfg.Endpoint)
	require.Empty(t, cfg.ObjectKey)
	require.True(t, cfg.NeedSTSRefresh(600))
	collector := NewSSAArtifactCollectorWithContext(context.Background(), "job-1", "attempt-1", "subtask-1")
	t.Cleanup(collector.Cleanup)
	reporter := &ScannerAgentReporter{TaskId: "job-1", RuntimeId: "attempt-1", ssaCollector: collector, ssaUploadCfg: cfg}
	var scanNode *ScanNode
	// The same empty collector is a required SSA artifact; missing transport
	// fails closed before any network access instead of silently dropping it.
	require.ErrorContains(t, scanNode.finalizeSSAArtifactUpload(context.Background(), reporter, &ScriptExecutionResult{Data: map[string]any{"program_name": "compiled", "risk_count": 0}}), "scannode not ready")
	for _, key := range []string{scannodeCompanyIDParamKey, scannodeTaskIDParamKey, scannodeAttemptIDParamKey, scannodeSSATicketRequiredParamKey, scannodeSSASkipMigrateParamKey} {
		invalid := validCompanyDispatchParams()
		delete(invalid, key)
		require.Error(t, validateCompanyDispatchParams("company-1", "job-1", "attempt-1", invalid), key)
	}
}
