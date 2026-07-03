package syntaxflow_scan

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// generateHeavyPHPSource builds a PHP program with n entry functions, each
// taking a parameter $p and flowing it through a chainDepth-deep call chain
// into sink(). The SyntaxFlow dataflow(include=...) native call then runs a
// recursive getTopDefs from each sink call back along the chain to the entry
// parameter — n sources x chainDepth depth of total dataflow work. This is the
// same breadth-x-depth shape that the production file-upload / SQLi rules
// exercise via dataflow(include=... * & $params ...); on moodle-scale (11k+
// sources) it hangs the scan without a per-rule budget. Here it is shrunk to a
// size that is still measurable but bounded so the test can run quickly.
func generateHeavyPHPSource(n, chainDepth int) string {
	var b strings.Builder
	b.WriteString("<?php\n")
	b.WriteString("function sink($x){ echo $x; }\n")
	for i := 0; i < chainDepth; i++ {
		fmt.Fprintf(&b, "function f%d($x){ return f%d($x); }\n", i, i+1)
	}
	fmt.Fprintf(&b, "function f%d($x){ return $x; }\n", chainDepth)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "function entry%d($p){ sink(f0($p)); }\n", i)
	}
	return b.String()
}

// prepareHeavyPHPProgram compiles a heavy PHP program into the test SSA database
// under progID and returns a cleanup that deletes it. Mirrors prepareTestProgram
// in api_test.go but generates a broad dataflow workload.
func prepareHeavyPHPProgram(t *testing.T, progID string, n, chainDepth int) func() {
	vf := filesys.NewVirtualFs()
	vf.AddFile("heavy/src/heavy.php", generateHeavyPHPSource(n, chainDepth))
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.PHP),
		ssaapi.WithProgramPath("heavy"),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	require.NotNil(t, prog)
	return func() { ssadb.DeleteProgram(ssadb.GetDB(), progID) }
}

// heavyDataflowRule mirrors the production dataflow(include=...) pattern (see
// php-core-upload.sf / php-mysql-inject.sf): match sink call args, then run a
// recursive dataflow that keeps paths reaching a function parameter. With many
// entry functions this is the heavy per-rule workload that a per-rule budget
// must bound. $params uses opcode:param (a confirmed SyntaxFlow filter) instead
// of a PHP-superglobal match to keep the rule self-contained.
const heavyDataflowRule = `desc(
	title: "heavy-dataflow-test",
	type: audit
)

*?{opcode:param} as $params
sink(* as $source)
$source<dataflow(include=<<<CODE
* & $params as $__next__
CODE)> as $high
alert $high
`

// errorCapture collects scan error-callback messages (thread-safe).
type errorCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (e *errorCapture) cb() errorCallback {
	return func(taskid, status, msg string, args ...any) {
		e.mu.Lock()
		e.msgs = append(e.msgs, fmt.Sprintf(msg, args...))
		e.mu.Unlock()
	}
}

func (e *errorCapture) has(substr string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// runHeavyScan starts a scan over the heavy program with the given per-rule
// budget and returns (elapsed, errors). It is bounded by hardBackstop so a
// regression that re-introduces the hang fails the test instead of stalling.
func runHeavyScan(t *testing.T, progID string, budget time.Duration, hardBackstop time.Duration) (time.Duration, *errorCapture) {
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
			ssaconfig.WithRuleInputRaw(heavyDataflowRule),
			ssaconfig.WithScanRuleTimeout(budget),
			WithErrorCallback(errs.cb()),
		)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(hardBackstop):
		cancel()
		t.Fatalf("StartScan (budget=%s) hung > %s; per-rule timeout is not bailing the heavy dataflow rule", budget, hardBackstop)
	}
	require.NoError(t, scanErr, "StartScan returned an error (budget=%s)", budget)
	return time.Since(start), errs
}

// TestStartScan_RuleTimeout_BailsHeavyRule is the unit-level guard for the
// large-project dataflow hang (see sf_dataflow.go nativeCallDataFlow NOTE and
// the moodle benchmark in ssaapi/docs/ssa-compilation-benchmark.md). It proves:
//   - with no budget, the heavy dataflow rule does real, measurable work; and
//   - with a small per-rule budget, the rule is bailed at the budget (the scan
//     emits the "hit per-rule budget" error callback) and finishes fast.
//
// Together these show the per-rule wall-clock budget (WithScanRuleTimeout) is
// the binding bound on total dataflow work that dataflowValueLimit/MaxDepth
// (per-branch only) do not provide.
func TestStartScan_RuleTimeout_BailsHeavyRule(t *testing.T) {
	const n, chainDepth = 2000, 15
	progID := uuid.NewString()
	cleanup := prepareHeavyPHPProgram(t, progID, n, chainDepth)
	defer cleanup()

	// Baseline: no per-rule budget. The heavy rule runs to completion and must
	// do real work (well over the budget below) but still finish (not hang).
	baselineElapsed, baselineErrs := runHeavyScan(t, progID, 0, 90*time.Second)
	require.False(t, baselineErrs.has("per-rule budget"),
		"baseline (no budget) should not be bailed by the per-rule budget")
	require.Greater(t, baselineElapsed, 150*time.Millisecond,
		"baseline heavy rule should do real work (> budget), took %s", baselineElapsed)

	// With a small per-rule budget, the heavy rule is bailed at the budget: the
	// scan emits the "hit per-rule budget" error callback and finishes fast.
	budgetElapsed, budgetErrs := runHeavyScan(t, progID, 100*time.Millisecond, 30*time.Second)
	require.True(t, budgetErrs.has("per-rule budget"),
		"budget scan should bail the heavy rule (expected 'per-rule budget' error callback), got: %v", budgetErrs.msgs)
	require.Less(t, budgetElapsed, 5*time.Second,
		"scan with 100ms per-rule budget should finish fast, took %s", budgetElapsed)
	require.Less(t, budgetElapsed, baselineElapsed,
		"budget scan (%s) should be faster than baseline (%s)", budgetElapsed, baselineElapsed)
}
