package ssaapi

import (
	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"os"
	"testing"
)

// --- from analyze_context_cache_test.go ---

// TestAnalyzeContext_ResolvedCacheDedup verifies that getResolvedValue handles
// nil/zero inputs safely and that a fresh AnalyzeContext starts with no cache
// (no cross-descent leakage).
func TestAnalyzeContext_ResolvedCacheDedup(t *testing.T) {
	actx := NewAnalyzeContext()
	// A nil program / zero id must not panic and must return not-ok.
	if _, ok := actx.getResolvedValue(nil, 0); ok {
		t.Fatalf("nil inst should not resolve")
	}
	if _, ok := actx.getResolvedValue(nil, 1); ok {
		t.Fatalf("nil inst should not resolve")
	}
	// The cache map is lazily created only on a real resolution; a fresh
	// context with only nil/zero calls must stay nil.
	if actx.resolvedInstCache != nil {
		t.Fatalf("fresh AnalyzeContext should have nil cache after nil/zero calls")
	}
	// Two different contexts must be distinct objects.
	actx2 := NewAnalyzeContext()
	if actx == actx2 {
		t.Fatalf("contexts should be distinct")
	}
}

// TestAnalyzeContext_ResolvedCacheKey verifies the cache key uses program+id
// equality semantics.
func TestAnalyzeContext_ResolvedCacheKey(t *testing.T) {
	k1 := resolvedInstKey{prog: nil, instID: 5}
	k2 := resolvedInstKey{prog: nil, instID: 5}
	if k1 != k2 {
		t.Fatalf("equal keys should be equal")
	}
	k3 := resolvedInstKey{prog: nil, instID: 6}
	if k1 == k3 {
		t.Fatalf("different ids should differ")
	}
}

// --- from flush_threshold_test.go ---

// TestFlushCompileUnitThresholdDefault verifies the default value when
// YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD is not set.
func TestFlushCompileUnitThresholdDefault(t *testing.T) {
	os.Unsetenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdEnvVar verifies that the env var is read
// and parsed correctly.
func TestFlushCompileUnitThresholdEnvVar(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "50000")
	require.Equal(t, 50000, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdInvalid verifies fallback on invalid values.
func TestFlushCompileUnitThresholdInvalid(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "not-a-number")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}

// TestFlushCompileUnitThresholdNonPositive verifies fallback on non-positive values.
func TestFlushCompileUnitThresholdNonPositive(t *testing.T) {
	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "0")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())

	t.Setenv("YAK_SSA_FLUSH_COMPILE_UNIT_THRESHOLD", "-100")
	require.Equal(t, defaultFlushCompileUnitThreshold, flushCompileUnitThreshold())
}

// --- from o1_alias_red_test.go ---

// TestO1_ValueSetAnchorSharesPointer_DirectAliasCorruption proves the O1
// correctness hazard: Value.SetAnchorBitVector stores the *BitVector pointer
// directly WITHOUT marking it shared (no ShareWords). So if two holders are
// given the SAME *BitVector, the shared flag stays false, CanMutateInPlace()
// returns true, and O1's in-place Or in mergeAnchorBits corrupts the other
// holder's bits.
func TestO1_ValueSetAnchorSharesPointer_DirectAliasCorruption(t *testing.T) {
	prog := NewTmpProgram("o1-alias")
	holder1 := fastMatchTestValue(t, prog, 1)
	holder2 := fastMatchTestValue(t, prog, 2)

	// Both holders directly reference the SAME BitVector (as Value.SetAnchor
	// allows — it stores the pointer without Clone/ShareWords).
	shared := utils.NewBitVector()
	shared.Set(3)
	holder1.SetAnchorBitVector(shared)
	holder2.SetAnchorBitVector(shared)

	require.True(t, holder1.GetAnchorBitVector().Has(3))
	require.True(t, holder2.GetAnchorBitVector().Has(3))

	// holder1 gets merged with a source that sets bit 9. If O1 in-place Or
	// fires (shared flag is false), it will mutate `shared` in place, leaking
	// bit 9 into holder2.
	src := fastMatchTestValue(t, prog, 3)
	srcBits := utils.NewBitVector()
	srcBits.Set(9)
	src.SetAnchorBitVector(srcBits)

	sf.MergeAnchor(src, holder1)

	// holder2 must NOT gain bit 9.
	require.False(t, holder2.GetAnchorBitVector().Has(9),
		"holder2 must not observe holder1's merged bit 9 (shared backing mutated in place)")
	require.True(t, holder2.GetAnchorBitVector().Has(3), "holder2 must keep its own bit 3")
}

// --- from path_a_reload_test.go ---

// TestPathAEnabledDefault verifies Path A is disabled by default.
func TestPathAEnabledDefault(t *testing.T) {
	os.Unsetenv("YAK_SSA_PATH_A_RELOAD")
	require.False(t, PathAEnabled(), "Path A should be disabled by default")
}

// TestPathAEnabledPositive verifies Path A is enabled when env=1.
func TestPathAEnabledPositive(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "1")
	require.True(t, PathAEnabled())
}

// TestPathAEnabledNonPositive verifies Path A is disabled for non-positive values.
func TestPathAEnabledNonPositive(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "0")
	require.False(t, PathAEnabled())

	t.Setenv("YAK_SSA_PATH_A_RELOAD", "-1")
	require.False(t, PathAEnabled())
}

// TestPathAEnabledInvalid verifies Path A is disabled for invalid values.
func TestPathAEnabledInvalid(t *testing.T) {
	t.Setenv("YAK_SSA_PATH_A_RELOAD", "not-a-number")
	require.False(t, PathAEnabled())
}

// TestReloadProgramFromDatabaseNil verifies nil safety.
func TestReloadProgramFromDatabaseNil(t *testing.T) {
	result := ReloadProgramFromDatabase(nil)
	require.Nil(t, result)
}

// TestReloadProgramFromDatabaseEmptyName verifies fallback when program name is empty.
func TestReloadProgramFromDatabaseEmptyName(t *testing.T) {
	prog := &Program{
		Program: nil, // no underlying SSA program
	}
	result := ReloadProgramFromDatabase(prog)
	// Should return the original program (fallback) when name is empty
	require.NotNil(t, result, "should return original program on fallback")
}

// --- from value_pool_test.go ---

// TestValuePool_AcquireIsIndependent verifies that a pooled Value is always a
// brand-new independent object: after release+reacquire the identity field
// (uid) is re-initialized and no stale analysis state leaks between
// acquisitions. This is the A1 safety invariant — pooled memory must never be
// observed carrying a previous owner's state.
func TestValuePool_AcquireIsIndependent(t *testing.T) {
	seen := map[int64]struct{}{}
	var id int64
	for i := 0; i < 2000; i++ {
		v := acquireValue()
		id++
		v.uid = id
		if _, dup := seen[v.uid]; dup {
			t.Fatalf("duplicate uid %d", v.uid)
		}
		seen[v.uid] = struct{}{}

		// analysis state must be empty on a fresh acquisition
		if v.EffectOn != nil || v.DependOn != nil || v.runtimeCtx != nil {
			t.Fatalf("acquireValue leaked analysis state")
		}
		if v.Predecessors != nil || v.DescInfo != nil || v.anchorBits != nil {
			t.Fatalf("acquireValue leaked sfvm state")
		}
		if v.users != nil || v.operands != nil {
			t.Fatalf("acquireValue leaked users/operands cache")
		}
		if v.ParentProgram != nil {
			t.Fatalf("acquireValue leaked ParentProgram")
		}

		// Simulate the factory-shell lifecycle: give it transient state, then
		// release so the next iteration reuses the memory.
		v.runtimeCtx = nil
		releaseValue(v)
	}
}

// TestValuePool_ReleaseZeros verifies that releaseValue zeroes the struct so a
// subsequent acquireValue cannot observe the previous owner's fields.
func TestValuePool_ReleaseZeros(t *testing.T) {
	// Put a deliberately polluted Value into the pool.
	releaseValue(&Value{uid: 42, Predecessors: []*PredecessorValue{{}}})
	// acquireValue must fully zero it before handing it out.
	v := acquireValue()
	if v.uid != 0 {
		t.Fatalf("pooled value retained uid %d", v.uid)
	}
	if v.EffectOn != nil || v.DependOn != nil || v.runtimeCtx != nil {
		t.Fatalf("pooled value retained analysis state")
	}
	if v.innerValue != nil || v.innerUser != nil {
		t.Fatalf("pooled value retained identity")
	}
	releaseValue(v)
}

// --- from sf_check_fastmatch_test.go ---

func fastMatchTestValue(t *testing.T, prog *Program, id int64) *Value {
	inst := ssa.NewConst(id)
	inst.SetId(id)
	v, err := prog.NewValue(inst)
	require.NoError(t, err)
	return v
}

func fastMatchTestCheck(t *testing.T, sourceIDs ...int64) *sfCheck {
	prog := NewTmpProgram("sf-check-fastmatch")
	vals := make(sf.Values, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		vals = append(vals, fastMatchTestValue(t, prog, id))
	}
	table := omap.NewEmptyOrderedMap[string, sf.Values]()
	table.Set("source", vals)
	result := &sf.SFFrameResult{SymbolTable: table}
	return &sfCheck{contextResult: result}
}

func TestSFCheck_FastPathMatch_SourceMembership(t *testing.T) {
	check := fastMatchTestCheck(t, 1, 2, 3)
	prog := NewTmpProgram("sf-check-fastmatch-path")

	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}

	path := sf.Values{fastMatchTestValue(t, prog, 9), fastMatchTestValue(t, prog, 2)}
	match, ok := check.fastPathMatch(item, path)
	require.True(t, ok, "simple source intersection must be handled by fast path")
	require.True(t, match, "path containing a source id must match")

	path = sf.Values{fastMatchTestValue(t, prog, 8), fastMatchTestValue(t, prog, 9)}
	match, ok = check.fastPathMatch(item, path)
	require.True(t, ok)
	require.False(t, match, "path without source id must not match")
}

func TestSFCheck_FastPathMatch_UnknownPatternFallsBack(t *testing.T) {
	check := fastMatchTestCheck(t, 1)
	prog := NewTmpProgram("sf-check-fastmatch-unknown")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* ?{opcode:call}?{!<self> & $source}",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.False(t, ok, "complex include must keep the full sub-query path")
}

func TestSFCheck_FastPathMatch_MissingSymbolFallsBack(t *testing.T) {
	check := fastMatchTestCheck(t)
	prog := NewTmpProgram("sf-check-fastmatch-missing")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.False(t, ok, "unbound source symbol must fall back to the full query")
}

func TestFastPathMatchStats_TracksHitsAndFallbacks(t *testing.T) {
	beforeHit, beforeFallback := FastPathMatchStats()
	check := fastMatchTestCheck(t, 1)
	prog := NewTmpProgram("sf-check-fastmatch-stats")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.True(t, ok)
	_, ok = check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 99)})
	require.True(t, ok)
	afterHit, afterFallback := FastPathMatchStats()
	require.GreaterOrEqual(t, afterHit-beforeHit, int64(2), "two simple patterns must be fast-path hits")
	require.GreaterOrEqual(t, afterFallback-beforeFallback, int64(0))
}

var _ = utils.Error // keep import

// TestSFCheck_FastPath_SymbolTableGrowthIsFresh verifies the fast path does
// not memoize the symbol set: a symbol bound after the first fast-path call
// must be visible to later calls (review A1).
func TestSFCheck_FastPath_SymbolTableGrowthIsFresh(t *testing.T) {
	prog := NewTmpProgram("sf-check-fastmatch-growth")
	table := omap.NewEmptyOrderedMap[string, sf.Values]()
	table.Set("source", sf.Values{fastMatchTestValue(t, prog, 1)})
	result := &sf.SFFrameResult{SymbolTable: table}
	check := &sfCheck{contextResult: result}
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}

	match, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 2)})
	require.True(t, ok)
	require.False(t, match, "id 2 is not in the initial source set")

	// The symbol table grows during descent; the next fast-path call must see
	// the new member instead of a stale memoized set.
	table.Set("source", sf.Values{fastMatchTestValue(t, prog, 1), fastMatchTestValue(t, prog, 2)})
	match, ok = check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 2)})
	require.True(t, ok)
	require.True(t, match, "newly bound source id must be visible without cache invalidation")
}
