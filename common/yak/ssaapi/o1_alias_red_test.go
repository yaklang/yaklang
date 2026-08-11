package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
)

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
