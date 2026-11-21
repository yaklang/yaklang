package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFlatOccultation_StopsOnPhiCycle covers the shape that made the KeyMatch
// search path unusable on large programs: a loop's phi points, through another
// phi, back at itself. The walk used to follow that cycle until the heap ran
// out (php-src: 2.1 GB live, 21 GB RSS, OOM-killed).
func TestFlatOccultation_StopsOnPhiCycle(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)
	blk := f.NewBasicBlock("entry")

	a := NewPhi(blk, "a")
	b := NewPhi(blk, "b")
	a.AddOccultation(b)
	b.AddOccultation(a)

	got := a.FlatOccultation()
	require.NotEmpty(t, got)
	seen := make(map[int64]struct{}, len(got))
	for _, v := range got {
		_, dup := seen[v.GetId()]
		require.False(t, dup, "a value must not be reported twice: %v", v)
		seen[v.GetId()] = struct{}{}
	}
	require.Len(t, got, 2, "the walk visits `b`, then comes back to `a` once")

	// A self reference must terminate on the first hop.
	c := NewPhi(blk, "c")
	c.AddOccultation(c)
	self := c.FlatOccultation()
	require.Len(t, self, 1)
	require.Equal(t, c.GetId(), self[0].GetId())
}

// TestFlatOccultation_KeepsChainOrder locks the behaviour the search path
// relies on: members of a nested phi come back after the phi itself, in one
// flat list.
func TestFlatOccultation_KeepsChainOrder(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)
	blk := f.NewBasicBlock("entry")

	leaf := NewPhi(blk, "leaf")
	inner := NewPhi(blk, "inner")
	inner.AddOccultation(leaf)
	outer := NewPhi(blk, "outer")
	outer.AddOccultation(inner)

	got := outer.FlatOccultation()
	require.Len(t, got, 2)
	require.Equal(t, inner.GetId(), got[0].GetId())
	require.Equal(t, leaf.GetId(), got[1].GetId())
}

// TestFlatOccultation_BoundsLongChain stops a chain that never repeats but is
// longer than any real phi graph, so one search term cannot walk forever.
func TestFlatOccultation_BoundsLongChain(t *testing.T) {
	prog := newHaveSubBlockTestProgram(t)
	f := newHaveSubBlockTestFunction(t, prog)
	blk := f.NewBasicBlock("entry")

	phis := make([]*Phi, flatOccultationMaxValues+64)
	for i := range phis {
		phis[i] = NewPhi(blk, "p")
	}
	for i := 0; i < len(phis)-1; i++ {
		phis[i].AddOccultation(phis[i+1])
	}

	got := phis[0].FlatOccultation()
	require.Len(t, got, flatOccultationMaxValues,
		"the walk must stop at the budget instead of materializing the chain")
}
