package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

// TestSFCheck_AppendItems_CompilesOncePerUniqueText asserts that sfCheck
// memoizes sub-rule compilation: appending the SAME text N times on one
// sfCheck must call the underlying VM.Compile ONCE (not N times).
//
// Without Opt B (compiledFrame memoization), every AppendItems call invokes
// c.vm.Compile(item.Value), recompiling + re-running the antlr parser/regexp
// for identical text on every dataflow native call. Across a scan, the same
// include/exclude text (`* & $params as $__next__`) is recompiled per rule —
// regexp compile showed up at ~10GB alloc on the large-project profile.
//
// The test drives a real sfCheck through CreateCheck + AppendItems (the
// production path) and reads the test-only VM.CompileCount() counter:
// RED before Opt B (count == N), GREEN after (count == 1).
func TestSFCheck_AppendItems_CompilesOncePerUniqueText(t *testing.T) {
	// CreateCheck needs an *sf.SFFrameResult and *sf.Config. Build minimal ones
	// directly via the sfvm package — AppendItems does not touch the context
	// result until an item is appended, so an empty rule is fine.
	cfg := sf.NewConfig()
	contextResult := sf.NewSFResult(nil, cfg)
	check := CreateCheck(contextResult, cfg)

	const dupText = "* & $params as $__next__"
	vmCountBefore := check.vm.CompileCount()

	const N = 5
	for i := 0; i < N; i++ {
		check.AppendItems(&sf.RecursiveConfigItem{
			Key:            string(sf.RecursiveConfig_Include),
			Value:          dupText,
			SyntaxFlowRule: true,
		})
	}
	compiles := check.vm.CompileCount() - vmCountBefore
	t.Logf("VM.Compile called %d times for %d duplicate AppendItems", compiles, N)
	// Before Opt B: compiles == N (each AppendItems recompiles). After Opt B:
	// compiles == 1 (memoized). Require the memoized count.
	require.Equal(t, int64(1), compiles,
		"duplicate AppendItems recompiled %d times (expected 1); sfCheck.compiledFrame memoization is not in effect", compiles)

	// A DIFFERENT text must compile exactly once more.
	const differentText = "* & $other as $__next__"
	beforeSecond := check.vm.CompileCount()
	check.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Include),
		Value:          differentText,
		SyntaxFlowRule: true,
	})
	secondCompiles := check.vm.CompileCount() - beforeSecond
	require.Equal(t, int64(1), secondCompiles,
		"a NEW unique text should compile exactly once, got %d", secondCompiles)

	// Re-appending the second text must NOT compile again (memo hit).
	beforeReuse := check.vm.CompileCount()
	check.AppendItems(&sf.RecursiveConfigItem{
		Key:            string(sf.RecursiveConfig_Include),
		Value:          differentText,
		SyntaxFlowRule: true,
	})
	reuseCompiles := check.vm.CompileCount() - beforeReuse
	require.Equal(t, int64(0), reuseCompiles,
		"re-appending an already-seen text should be a memo hit (0 compiles), got %d", reuseCompiles)

	// Sanity: the compiled frames are usable (non-nil) — guards against the
	// memo returning a stale/nil frame.
	require.NotEmpty(t, check.matchItem, "matchItem should have items after AppendItems")
	for _, item := range check.matchItem {
		require.NotNil(t, item.frame, "memoized frame must not be nil")
	}
}
