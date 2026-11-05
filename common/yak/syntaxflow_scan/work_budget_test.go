package syntaxflow_scan

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// heavyTopDefRule drives the SF <getTopDef> opcode (`#->`) over every sink call
// arg. Each arg's GetTopDefs descends the f0->f1->...->entry-param call chain
// (chainDepth nodes), so n sources x chainDepth depth of recursive descent —
// the same getTopDefs hot path the production XSS / path-traversal rules
// exercise (see frame_exec.go OpGetTopDefs -> GetSyntaxFlowTopDef ->
// DataFlowWithSFConfig -> Value.GetTopDefs -> AnalyzeContext.check). On
// moodle/javacms-scale (7k+ sources) this is exactly the within-opcode fanout
// that hangs for hours under the 4h per-rule wall-clock backstop. Here it is
// shrunk to a bounded-but-measurable size.
const heavyTopDefRule = `desc(
	title: "heavy-topdef-test",
	type: audit
)

*?{opcode:param} as $params
sink(* as $source)
$source #-> as $high
alert $high
`

// runHeavyScanWithWorkLimit starts a scan over the heavy program with a per-rule
// total-work budget (and a generous wall-clock so the WORK budget is what
// bails, not the timeout) and returns (elapsed, errors). hardBackstop fails the
// test instead of stalling on a regression that re-introduces the hang.
func runHeavyScanWithWorkLimit(t *testing.T, progID string, workLimit int64, ruleTimeout, hardBackstop time.Duration) (time.Duration, *errorCapture) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := &errorCapture{}
	var scanErr error
	done := make(chan struct{})
	start := time.Now()
	go func() {
		scanErr = StartScan(ctx,
			ssaconfig.WithProgramNames(progID),
			ssaconfig.WithRuleInputRaw(heavyTopDefRule),
			ssaconfig.WithScanRuleTimeout(ruleTimeout),
			ssaconfig.WithScanRuleWorkLimit(workLimit),
			WithErrorCallback(errs.cb()),
		)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(hardBackstop):
		cancel()
		t.Fatalf("StartScan (workLimit=%d, timeout=%s) hung > %s; per-rule work budget is not bailing the heavy getTopDef descent", workLimit, ruleTimeout, hardBackstop)
	}
	require.NoError(t, scanErr, "StartScan returned an error (workLimit=%d)", workLimit)
	return time.Since(start), errs
}

// TestStartScan_WorkBudget_BailsHeavyTopDefRule is the unit-level guard for the
// large-project getTopDef hang (see plan-expressive-pebble.md Fix 1). It proves:
//   - with no work budget (workLimit=0, generous timeout), the heavy getTopDef
//     rule runs to completion and does real, measurable work; and
//   - with a small per-rule work budget, the rule is bailed at the budget (the
//     scan emits the "per-rule budget" error callback) and finishes fast —
//     without the per-element EnterWork() in AnalyzeContext.check() the budget
//     counter would never increment and the low-limit scan would hang.
//
// Together these show the total-work budget (WithScanRuleWorkLimit) is the
// binding structural bound on within-opcode getTopDef fanout that the wall-clock
// RuleTimeout only catches after the fact.
func TestStartScan_WorkBudget_BailsHeavyTopDefRule(t *testing.T) {
	const n, chainDepth = 200, 5
	progID := uuid.NewString()
	cleanup := prepareHeavyPHPProgram(t, progID, n, chainDepth)
	defer cleanup()

	// Baseline: no work budget, generous wall-clock. The heavy rule runs to
	// completion and must do real work (well over the budget below) but finish.
	baselineElapsed, baselineErrs := runHeavyScanWithWorkLimit(t, progID, 0, 5*time.Minute, 120*time.Second)
	require.False(t, baselineErrs.has("per-rule budget"),
		"baseline (no work budget) should not be bailed by the per-rule budget, got: %v", baselineErrs.msgs)
	require.Greater(t, baselineElapsed, 20*time.Millisecond,
		"baseline heavy rule should do real work, took %s", baselineElapsed)

	// With a small work budget (500 ops << n*chainDepth=1000 descent nodes),
	// the rule is bailed at the budget: the scan emits the "per-rule budget"
	// error callback and finishes fast. The wall-clock (5m) is generous so the
	// WORK budget is what fires, proving the per-element EnterWork() in
	// AnalyzeContext.check() is in effect.
	budgetElapsed, budgetErrs := runHeavyScanWithWorkLimit(t, progID, 500, 5*time.Minute, 60*time.Second)
	require.True(t, budgetErrs.has("per-rule budget"),
		"work-budget scan should bail the heavy rule (expected 'per-rule budget' error callback), got: %v", budgetErrs.msgs)
	require.True(t, budgetErrs.has("visited="),
		"work-budget diagnostic should include actual usage, got: %v", budgetErrs.msgs)
	require.Less(t, budgetElapsed, 30*time.Second,
		"scan with workLimit=500 should finish fast, took %s", budgetElapsed)
	require.Less(t, budgetElapsed, baselineElapsed,
		"work-budget scan (%s) should be faster than baseline (%s)", budgetElapsed, baselineElapsed)
}

// TestStartScan_WorkBudget_BailIsPartialNotFatal asserts that a work-budget
// bail surfaces as a partial-result bail (the scan completes, StartScan returns
// nil, the rule is logged as hit-budget) rather than a fatal scan failure. This
// guards the runtime.go bailedByBudget path that recognizes workBudget.Exceeded().
func TestStartScan_WorkBudget_BailIsPartialNotFatal(t *testing.T) {
	const n, chainDepth = 200, 5
	progID := uuid.NewString()
	cleanup := prepareHeavyPHPProgram(t, progID, n, chainDepth)
	defer cleanup()

	// Tiny work budget so the heavy rule bails almost immediately. StartScan
	// must still return nil (partial bail, not a fatal error) and emit the
	// per-rule budget callback.
	elapsed, errs := runHeavyScanWithWorkLimit(t, progID, 100, 5*time.Minute, 60*time.Second)
	require.True(t, errs.has("per-rule budget"),
		"work-budget bail should emit 'per-rule budget' callback, got: %v", errs.msgs)
	require.Less(t, elapsed, 30*time.Second,
		"work-budget bail should finish fast, took %s", elapsed)
}

func TestEffectiveRuleWorkLimitAutoScale(t *testing.T) {
	defaultLimit := ssaconfig.DefaultScanRuleWorkLimit
	require.Equal(t, defaultLimit, effectiveRuleWorkLimit(defaultLimit, 100_000),
		"small projects keep the CLI default")
	require.Equal(t, defaultLimit*3, effectiveRuleWorkLimit(defaultLimit, 1_000_000),
		"large projects scale by source lines")
	require.Equal(t, int64(1_000_000), effectiveRuleWorkLimit(defaultLimit, 20_000_000),
		"auto-scale is capped")
	require.Equal(t, int64(5_000), effectiveRuleWorkLimit(5_000, 5_000_000),
		"non-default explicit limits are never auto-scaled")
	require.Equal(t, int64(0), effectiveRuleWorkLimit(0, 5_000_000),
		"disabled work limit stays disabled")
}

// TestStartScan_DefaultWorkBudget_BailsHeavyTopDefRule guards the scan-entry
// DEFAULT: a caller that passes no budget options at all (the library/gRPC
// path, e.g. a platform-driven ScanProject) must still get the CLI-default
// per-rule work budget. The workload (n sources x chainDepth-deep call chains
// > DefaultScanRuleWorkLimit descent nodes) exceeds the default, so the rule
// must bail at the budget with partial results instead of running unbounded —
// the gap that let one topdef(include=*) rule hang a large-project scan for
// 71+ minutes with zero budget log lines.
func TestStartScan_DefaultWorkBudget_BailsHeavyTopDefRule(t *testing.T) {
	const n, chainDepth = 1000, 60 // ~60k descent nodes > 50k default limit
	progID := uuid.NewString()
	cleanup := prepareHeavyPHPProgram(t, progID, n, chainDepth)
	defer cleanup()

	errs := &errorCapture{}
	var scanErr error
	done := make(chan struct{})
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// No WithScanRuleWorkLimit / WithScanRuleTimeout: the defaults must apply.
		scanErr = StartScan(ctx,
			ssaconfig.WithProgramNames(progID),
			ssaconfig.WithRuleInputRaw(heavyTopDefRule),
			WithErrorCallback(errs.cb()),
		)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		cancel()
		t.Fatalf("StartScan (default budgets) hung > 3m; the default per-rule work budget is not bounding the heavy getTopDef descent")
	}
	require.NoError(t, scanErr)
	require.True(t, errs.has("per-rule budget"),
		"default-budget scan should bail the heavy rule (expected 'per-rule budget' error callback), got: %v", errs.msgs)
	require.True(t, errs.has(fmt.Sprintf("work-limit=%d", ssaconfig.DefaultScanRuleWorkLimit)),
		"default-budget bail should report the default work limit, got: %v", errs.msgs)
	require.Less(t, time.Since(start), 2*time.Minute,
		"default-budget bail should finish well under the hard backstop")
}
