package ssaapi

import (
	"testing"
)

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
