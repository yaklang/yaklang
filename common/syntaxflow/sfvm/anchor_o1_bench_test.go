package sfvm

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

// BenchmarkMergeAnchor_O1_UniqueOwnerOrInPlace measures the O1 target path: a
// destination that already has a unique-owner bitvector (fresh, not aliased)
// being merged from a source. O1 makes this Or in place (0 alloc); the old
// Clone+Or path allocated on every merge (the 26.1GB hadoop hotspot).
func BenchmarkMergeAnchor_O1_UniqueOwnerOrInPlace(b *testing.B) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(7)
	src.SetAnchorBitVector(srcBits)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst := newDirectAnchorValue("dst")
		dstBits := utils.NewBitVector()
		dstBits.Set(3)
		dst.SetAnchorBitVector(dstBits)
		MergeAnchor(src, dst)
	}
}

// BenchmarkMergeAnchor_O1_SharedOwnerDetaches measures the still-shared path:
// a destination whose bitvector is aliased must still detach (correctness), so
// it documents the retained allocation for shared vectors.
func BenchmarkMergeAnchor_O1_SharedOwnerDetaches(b *testing.B) {
	src := newDirectAnchorValue("src")
	srcBits := utils.NewBitVector()
	srcBits.Set(9)
	src.SetAnchorBitVector(srcBits)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alias := utils.NewBitVector()
		alias.Set(3)
		shared := alias.Clone()
		dst := newDirectAnchorValue("dst")
		dst.SetAnchorBitVector(shared)
		MergeAnchor(src, dst)
	}
}
