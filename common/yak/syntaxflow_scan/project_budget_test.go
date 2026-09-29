package syntaxflow_scan

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestProjectStagePreservesRuleBudgets(t *testing.T) {
	for _, limit := range []int64{0, 12345} {
		root, err := ssaconfig.New(ssaconfig.ModeAll,
			ssaconfig.WithScanRuleWorkLimit(limit), ssaconfig.WithScanRuleTimeout(time.Duration(limit)*time.Millisecond))
		require.NoError(t, err)
		cfg := &Config{Config: root, ScanTaskCallback: &ScanTaskCallback{}}
		nested, err := ssaconfig.New(ssaconfig.ModeAll, sharedScanCallbackOptions(cfg)...)
		require.NoError(t, err)
		require.Equal(t, root.GetScanRuleWorkLimit(), nested.GetScanRuleWorkLimit())
		require.Equal(t, root.GetScanRuleTimeout(), nested.GetScanRuleTimeout())
	}
}

func TestScanProjectEnforcesRuleWorkBudget(t *testing.T) {
	name := uuid.NewString()
	cleanup := prepareHeavyPHPProgram(t, name, 200, 5)
	defer cleanup()
	for _, limit := range []int64{0, 500} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := ScanProject(ctx,
			ssaconfig.WithProgramNames(name), WithMode(SSAMode),
			ssaconfig.WithRuleInputRaw(heavyTopDefRule),
			ssaconfig.WithScanRuleTimeout(5*time.Minute),
			ssaconfig.WithScanRuleWorkLimit(limit))
		cancel()
		require.NoError(t, err)
		require.False(t, result.IncompleteStages, "a budget bail keeps the findings and does not fail the scan")
	}
}
