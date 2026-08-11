package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// directAnchorValue is a ValueOperator mock whose SetAnchorBitVector stores the
// pointer DIRECTLY (no defensive clone), matching ssaapi.Value's post-31e95a49b
// contract. This lets the COW in mergeAnchorBits be observed: a first-branch
// merge shares the source's bitvector pointer instead of cloning it (the
// ~355GB/35%-of-alloc saving). The existing bitVectorValue mock clones on Set,
// which would mask the COW behavior.
type directAnchorValue struct {
	stubValueOperator
	name string
	bits *utils.BitVector
}

func newDirectAnchorValue(name string) *directAnchorValue {
	return &directAnchorValue{name: name}
}

func (v *directAnchorValue) String() string { return v.name }
func (v *directAnchorValue) IsEmpty() bool  { return v == nil }
func (v *directAnchorValue) GetAnchorBitVector() *utils.BitVector {
	if v == nil || v.bits == nil {
		return nil
	}
	return v.bits
}
func (v *directAnchorValue) SetAnchorBitVector(bits *utils.BitVector) {
	if v == nil {
		return
	}
	v.bits = bits // direct store, no clone — mirrors ssaapi.Value
}

// TestMergeAnchorBits_COW_FirstBranchSharesPointer asserts the COW optimization
// in mergeAnchorBits: when dst has no anchor bits, the first merge stores the
// source's bitvector pointer directly (no Clone). This is the alloc saving —
// BitVector.Clone was 355GB / 35% of alloc on javacms-core, 99% from this
// branch. Before COW: dst gets a clone (different pointer). After COW: dst
// shares src's pointer.
func TestMergeAnchorBits_COW_FirstBranchSharesPointer(t *testing.T) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(3)
	srcBits.Set(7)
	src.SetAnchorBitVector(srcBits)

	dst := newDirectAnchorValue("dst") // fresh, no bits

	MergeAnchor(src, dst)

	// COW: dst shares src's bitvector pointer (no Clone).
	require.Same(t, src.GetAnchorBitVector(), dst.GetAnchorBitVector(),
		"COW first branch should share the source bitvector pointer, not clone")
	// And the content is visible on dst (same pointer → same bits).
	require.True(t, dst.GetAnchorBitVector().Has(3))
	require.True(t, dst.GetAnchorBitVector().Has(7))
}

// TestMergeAnchorBits_COW_SecondBranchDoesNotMutateSource asserts the
// clone-before-Or invariant that makes COW safe: a subsequent merge into dst
// (2nd branch) clones dst.bits (== the shared source pointer) before Or'ing, so
// the original source's bits are never mutated in place. If any future code
// path mutates anchor bits in place without cloning, this test goes red.
func TestMergeAnchorBits_COW_SecondBranchDoesNotMutateSource(t *testing.T) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(3)
	srcBits.Set(7)
	src.SetAnchorBitVector(srcBits)

	dst := newDirectAnchorValue("dst")
	MergeAnchor(src, dst) // first branch: dst.bits = srcBits (shared pointer)

	// Second merge into dst from another source with bit 1 set. The 2nd branch
	// must clone dst.bits (== srcBits) before Or'ing otherBits in, so srcBits
	// is NOT mutated.
	other := newDirectAnchorValue("other")
	otherBits := utils.NewBitVector()
	otherBits.Set(1)
	other.SetAnchorBitVector(otherBits)
	MergeAnchor(other, dst)

	// src's bits must be unchanged: still has 3 and 7, and does NOT have 1
	// (bit 1 was only in other; an in-place Or would have leaked it into src).
	require.True(t, src.GetAnchorBitVector().Has(3), "src bit 3 must survive")
	require.True(t, src.GetAnchorBitVector().Has(7), "src bit 7 must survive")
	require.False(t, src.GetAnchorBitVector().Has(1),
		"src must not gain other's bit 1 — 2nd-branch Or must clone dst.bits, not mutate the shared source")

	// dst no longer shares src's pointer (the 2nd branch replaced it with a clone).
	require.NotSame(t, src.GetAnchorBitVector(), dst.GetAnchorBitVector(),
		"2nd branch should have cloned dst.bits, so dst no longer shares src's pointer")
	// dst now carries src|other bits (3, 7, 1).
	require.True(t, dst.GetAnchorBitVector().Has(1))
	require.True(t, dst.GetAnchorBitVector().Has(3))
	require.True(t, dst.GetAnchorBitVector().Has(7))
}

// TestMergeAnchorBits_COW_EmptySourceIsNoop guards the early-return: merging an
// empty/nil source bitvector is a no-op (dst unchanged, no allocation).
func TestMergeAnchorBits_COW_EmptySourceIsNoop(t *testing.T) {
	src := newDirectAnchorValue("src") // no bits set
	dst := newDirectAnchorValue("dst")
	dstBits := utils.NewBitVector()
	dstBits.Set(5)
	dst.SetAnchorBitVector(dstBits)

	MergeAnchor(src, dst)

	require.Same(t, dstBits, dst.GetAnchorBitVector(), "empty-source merge should leave dst untouched")
	require.True(t, dst.GetAnchorBitVector().Has(5))
}

// TestMergeAnchorBits_O1_UniqueOwnerOrInPlace asserts the O1 optimization: when
// dst already has a non-empty bitvector that is a UNIQUE owner (freshly built,
// not aliased via Clone/COW), mergeAnchorBits must Or in place and keep the same
// pointer (zero allocation) instead of the current Clone()-then-Or() that
// detaches and allocates. This is the 26.1GB Or-detach hotspot on hadoop.
func TestMergeAnchorBits_O1_UniqueOwnerOrInPlace(t *testing.T) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(7)
	src.SetAnchorBitVector(srcBits)

	// dst already has a unique-owner non-empty bitvector that does NOT contain
	// src's bit (so the idempotent-Contains short-circuit does not fire).
	dst := newDirectAnchorValue("dst")
	dstBits := utils.NewBitVector()
	dstBits.Set(3)
	dst.SetAnchorBitVector(dstBits)
	require.True(t, dstBits.CanMutateInPlace(), "precondition: dstBits is a unique owner")

	MergeAnchor(src, dst)

	// O1: dst's bitvector pointer must be unchanged (Or in place, no clone).
	require.Same(t, dstBits, dst.GetAnchorBitVector(),
		"unique-owner dstBits should be Or'd in place, pointer unchanged (0 alloc)")
	// Result bits must be the union (bit-equivalence preserved).
	require.True(t, dst.GetAnchorBitVector().Has(3))
	require.True(t, dst.GetAnchorBitVector().Has(7))
	// Source must be untouched.
	require.True(t, src.GetAnchorBitVector().Has(7))
	require.False(t, src.GetAnchorBitVector().Has(3))
}

// TestMergeAnchorBits_O1_SharedOwnerMustNotMutateInPlace asserts the negative
// guard: when dst's bitvector is SHARED (aliased via Clone), mergeAnchorBits
// must NOT Or in place — it must detach so the shared alias is not corrupted.
func TestMergeAnchorBits_O1_SharedOwnerMustNotMutateInPlace(t *testing.T) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(9)
	src.SetAnchorBitVector(srcBits)

	// dst's bitvector is a Clone (shared with an alias) that does NOT contain
	// src's bit.
	alias := utils.NewBitVector()
	alias.Set(3)
	sharedDstBits := alias.Clone() // shared with `alias`
	dst := newDirectAnchorValue("dst")
	dst.SetAnchorBitVector(sharedDstBits)
	require.False(t, sharedDstBits.CanMutateInPlace(), "precondition: sharedDstBits is NOT a unique owner")

	MergeAnchor(src, dst)

	// The alias must NOT gain src's bit (shared vector must detach, not mutate).
	require.False(t, alias.Has(9), "shared alias must not be mutated in place")
	// dst gets the merged result.
	require.True(t, dst.GetAnchorBitVector().Has(3))
	require.True(t, dst.GetAnchorBitVector().Has(9))
}
