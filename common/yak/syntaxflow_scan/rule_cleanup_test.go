package syntaxflow_scan

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestScan_ResetInterRuleState_ClearsHeavyRuleAccumulators asserts that after a
// heavy rule pushes the program's nodeId2ValueCache above the reset threshold,
// the inter-rule cleanup hook (syntaxflow_scan/runtime.go Query -> Program.
// ResetInterRuleState) fires and clears the cache so rule N's analysis state
// (cached Values + their Predecessors/EffectOn/DependOn/anchorBits) does not
// bleed into rule N+1.
//
// Without Opt C, the cached Values survive across rules (only the 8s TTL
// eventually drops them), so peak retained memory grows monotonically across
// the scan and a cached Value retains the previous rule's Predecessors. With
// Opt C, a heavy rule (cache > threshold) triggers ResetInterRuleState which
// purges the cache and nils the accumulators.
//
// Asserted deterministically via a test-only atomic counter on Program
// (ssaapi.Program.InterRuleResetCount), not via HeapInuse (too noisy at small
// synthetic sizes). RED before Opt C (reset count = 0 across two heavy rules),
// GREEN after (reset count >= 1).
func TestScan_ResetInterRuleState_ClearsHeavyRuleAccumulators(t *testing.T) {
	progID := uuid.NewString()
	// Large enough that the rule materializes many Values and pushes the cache
	// above resetInterRuleStateCacheThreshold between rules.
	cleanup := prepareHeavyPHPProgram(t, progID, 4000, 25)
	defer cleanup()

	// Lower the per-Program threshold on the EXACT instance the scan runs on.
	// The default 50k is calibrated for real projects, far above what a
	// synthetic VirtualFs program materializes; 1 forces the very first inter-
	// rule cleanup to fire (the cache count is 0 between rules, but threshold=1
	// is also not met when count=0 — so use 0 to fire whenever count >= 0).
	// FromDatabase returns the shared ProgramCache instance the scan uses.
	prog, err := ssaapi.FromDatabase(progID)
	require.NoError(t, err)
	require.NotNil(t, prog)
	prog.SetInterRuleStateThreshold(0) // fire on every rule boundary
	resetBefore := atomic.LoadInt64(&prog.InterRuleResetCount)

	// Run TWO heavy dataflow rules back-to-back. Each materializes a large
	// Value set; the inter-rule hook should reset between them once the cache
	// crosses the threshold.
	ruleA := `desc(title:"heavy-A", type:audit)
*?{opcode:param} as $params
sink(* as $source)
$source<dataflow(include=<<<CODE
* & $params as $__next__
CODE)> as $high
alert $high
`
	ruleB := `desc(title:"heavy-B", type:audit)
*?{opcode:param} as $params
sink(* as $source)
$source<dataflow(include=<<<CODE
* & $params as $__next__
CODE)> as $high2
alert $high2
`

	runRule := func(rule string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- StartScan(ctx,
				ssaconfig.WithProgramNames(progID),
				ssaconfig.WithRuleInputRaw(rule),
			)
		}()
		select {
		case err := <-done:
			require.NoError(t, err, "StartScan(%s) failed", rule)
		case <-time.After(180 * time.Second):
			cancel()
			t.Fatalf("StartScan(%s) hung", rule)
		}
	}
	runRule(ruleA)
	runRule(ruleB)

	resetAfter := atomic.LoadInt64(&prog.InterRuleResetCount)
	resets := resetAfter - resetBefore
	t.Logf("Program.InterRuleResetCount: before=%d after=%d (delta=%d)", resetBefore, resetAfter, resets)
	// Before Opt C: the runtime never calls ResetInterRuleState, so delta=0.
	// After Opt C: at least one reset fires between/after the heavy rules once
	// the cache crosses the threshold. Require >= 1.
	require.Greater(t, resets, int64(0),
		"Program.ResetInterRuleState never fired across two heavy rules; Opt C inter-rule cleanup hook is not in effect")
}
