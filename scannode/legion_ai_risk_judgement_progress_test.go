package scannode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func enableRiskJudgementProgress(t *testing.T, runtime *legionServerFocusRuntime) {
	t.Helper()
	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
	contract := testLegionRiskJudgementExecutionContract(t)
	contract.Capabilities = append(contract.Capabilities, serverFocusCapabilityRiskJudgementProgressV1)
	contract.capabilitySet[serverFocusCapabilityRiskJudgementProgressV1] = struct{}{}
	require.NoError(t, runtime.activateFocusTurn(runtime.authorizedFocusReleaseID, contract))
}

func TestLegionRiskJudgementProgressTracksNativeReceipts(t *testing.T) {
	runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())
	// Session binding injects the proxy, not the concrete native sink.
	runtime.sink = newAISessionResultSinkProxy(runtime.sink)
	enableRiskJudgementProgress(t, runtime)
	read := func(accepted, remaining []string) map[string]any {
		t.Helper()
		progress, err := runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, nil)
		require.NoError(t, err)
		require.Equal(t, map[string]any{
			"required_result_count": 2,
			"accepted_risk_ids":     accepted,
			"remaining_risk_ids":    remaining,
		}, progress)
		return progress
	}
	initial := read([]string{}, []string{"risk-1", "risk-2"})
	require.Empty(t, publisher.reports, "reading progress must not publish a result")
	initial["remaining_risk_ids"].([]string)[0] = "forged-risk"
	read([]string{}, []string{"risk-1", "risk-2"})

	_, err := runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1"))
	require.NoError(t, err)
	partial := read([]string{"risk-1"}, []string{"risk-2"})
	partial["accepted_risk_ids"].([]string)[0] = "forged-risk"
	read([]string{"risk-1"}, []string{"risk-2"})

	_, err = runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1"))
	require.NoError(t, err)
	read([]string{"risk-1"}, []string{"risk-2"})
	_, err = runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-2"))
	require.NoError(t, err)
	read([]string{"risk-1", "risk-2"}, []string{})
	_, err = runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("outside"))
	require.Error(t, err)
	read([]string{"risk-1", "risk-2"}, []string{})
}

func TestResilienceRiskJudgementProgressProxyRebind(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	_, oldSink, publisher := newTestRiskJudgementRuntime(t, resultContext)
	proxy := newAISessionResultSinkProxy(oldSink)
	value, err := newLegionServerFocusRuntime(context.Background(), resultContext.TargetUrl, proxy)
	require.NoError(t, err)
	runtime := value.(*legionServerFocusRuntime)
	runtime.authorizedFocusReleaseID = resultContext.FocusReleaseId
	enableRiskJudgementProgress(t, runtime)
	_, err = runtime.Execute(serverFocusCapabilitySubmitRiskJudgementV1, validRiskJudgementParams("risk-1"))
	require.NoError(t, err)
	progress, err := runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"risk-1"}, progress["accepted_risk_ids"])

	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
	setTestRiskJudgementScopeRiskIDs(resultContext, []string{"next-risk"})
	replacement, err := newLegionAIFocusResultSink(publisher, "bind-next", resultContext)
	require.NoError(t, err)
	proxy.Set(replacement)
	_, err = runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, nil)
	require.Error(t, err, "rebind must not reopen a dormant Turn")
	enableRiskJudgementProgress(t, runtime)
	progress, err = runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, progress["required_result_count"])
	require.Empty(t, progress["accepted_risk_ids"], "old receipts must not leak into the replacement sink")
	require.Equal(t, []string{"next-risk"}, progress["remaining_risk_ids"])
	runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)

	proxy.Set(&immediateAIFocusResultSink{})
	_, err = proxy.riskJudgementProgress()
	require.ErrorContains(t, err, "does not provide native risk judgement progress")
	var absent *aiSessionResultSinkProxy
	_, err = absent.riskJudgementProgress()
	require.ErrorContains(t, err, "unavailable")
}

func TestLegionRiskJudgementProgressRejectsUntrustedScope(t *testing.T) {
	runtime, _, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())
	enableRiskJudgementProgress(t, runtime)
	for _, params := range []map[string]any{
		{"required_result_count": 0},
		{"risk_ids": []string{"outside"}},
		{"accepted_risk_ids": []string{"risk-1", "risk-2"}},
	} {
		progress, err := runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, params)
		require.ErrorContains(t, err, "accepts no parameters")
		require.Nil(t, progress)
	}
	require.Empty(t, publisher.reports)
}

func TestResilienceRiskJudgementProgressRequiresAuthorizedTurn(t *testing.T) {
	for _, scenario := range []string{"undeclared", "inactive", "wrong-release", "missing-result-capability", "missing-result-contract", "missing-scope"} {
		t.Run(scenario, func(t *testing.T) {
			runtime, sink, publisher := newTestRiskJudgementRuntime(t, validAIRiskJudgementResultContext())
			if scenario != "undeclared" {
				enableRiskJudgementProgress(t, runtime)
			}
			switch scenario {
			case "inactive":
				runtime.deactivateFocusTurn(runtime.authorizedFocusReleaseID)
			case "wrong-release":
				runtime.activeFocusReleaseID = "other-release"
			case "missing-result-capability":
				delete(runtime.activeExecutionContract.capabilitySet, serverFocusCapabilitySubmitRiskJudgementV1)
			case "missing-result-contract":
				runtime.activeExecutionContract.Results = nil
				delete(runtime.activeExecutionContract.resultByCap, serverFocusCapabilitySubmitRiskJudgementV1)
			case "missing-scope":
				sink.riskJudgementScope = nil
			}
			progress, err := runtime.Execute(serverFocusCapabilityRiskJudgementProgressV1, nil)
			require.Error(t, err)
			require.Nil(t, progress)
			require.Empty(t, publisher.reports)
		})
	}
}
