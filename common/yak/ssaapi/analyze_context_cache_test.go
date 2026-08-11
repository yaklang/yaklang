package ssaapi

import (
	"testing"
)

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
