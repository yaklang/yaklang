package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestMergeBusinessFunctionResults_AccumulatesScopePaths(t *testing.T) {
	accum := &Phase1BusinessFunctionResult{
		ClassificationStrategy: "by_maven_module",
		Functions: map[string]BusinessFunctionEntry{
			"订单域": {
				ScopePaths: []string{"order-service/src/main/java/com/acme/order"},
			},
		},
	}
	delta := &Phase1BusinessFunctionResult{
		Functions: map[string]BusinessFunctionEntry{
			"订单域": {
				ScopePaths: []string{"order-service/src/main/java/com/acme/order/api"},
			},
			"支付域": {
				ScopePaths: []string{"payment-service/src/main/java/com/acme/payment"},
			},
		},
	}
	merged := mergeBusinessFunctionResults(accum, delta)
	require.Len(t, merged.Functions, 2)
	require.Len(t, merged.Functions["订单域"].ScopePaths, 2)
	require.Contains(t, merged.Functions["订单域"].ScopePaths, "order-service/src/main/java/com/acme/order/api")
}

func TestMergeBusinessFunctionResults_CoverageNeverRegresses(t *testing.T) {
	fixture := filepath.Join("testfixtures", "multi_module_maven")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)
	rt := &Runtime{Session: &store.DiscoverySession{CodeRootPath: abs, CodePathOK: true, Language: "java"}}
	inv, err := BuildJavaBusinessScopeInventory(rt)
	require.NoError(t, err)

	round1 := mergeBusinessFunctionResults(nil, &Phase1BusinessFunctionResult{
		Functions: map[string]BusinessFunctionEntry{
			"订单": {ScopePaths: []string{"order-service/src/main/java/com/acme/order"}},
		},
	})
	paths1 := collectScopePathsFromFunctionMapPayload(round1.Functions)
	cov1 := evaluateJavaBusinessCoverage(inv, paths1)
	require.False(t, cov1.Complete)
	require.Greater(t, cov1.Covered, 0)

	// Round 2 submits unrelated/wrong block only — merged map must retain round1 coverage.
	round2 := mergeBusinessFunctionResults(round1, &Phase1BusinessFunctionResult{
		Functions: map[string]BusinessFunctionEntry{
			"噪声": {ScopePaths: []string{"nonexistent/module/path"}},
		},
	})
	paths2 := collectScopePathsFromFunctionMapPayload(round2.Functions)
	cov2 := evaluateJavaBusinessCoverage(inv, paths2)
	require.GreaterOrEqual(t, cov2.Covered, cov1.Covered)

	round3 := mergeBusinessFunctionResults(round2, &Phase1BusinessFunctionResult{
		Functions: map[string]BusinessFunctionEntry{
			"支付": {ScopePaths: []string{"payment-service/src/main/java/com/acme/payment"}},
			"公共": {ScopePaths: []string{"common-lib/src/main/java/com/acme/common"}},
		},
	})
	paths3 := collectScopePathsFromFunctionMapPayload(round3.Functions)
	cov3 := evaluateJavaBusinessCoverage(inv, paths3)
	require.True(t, cov3.Complete, cov3.Feedback)
	require.Equal(t, cov3.Covered-cov2.Covered, countNewlyCoveredUnits(inv, paths2, paths3))
}
