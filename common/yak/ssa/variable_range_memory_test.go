package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

var rangeVariableSink *Variable

func TestVariableRangeEmptyAllocation(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		rangeVariableSink = NewVariable(1, "x", false, nil).(*Variable)
	})
	t.Logf("empty variable: %.0f allocations", allocs)
	require.LessOrEqual(t, allocs, float64(3), "a variable with no uses must not allocate a range map")
}

func TestVariableRangesDeduplicatePositionsAndShare(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	b := prog.GetAndCreateFunctionBuilder("", "ranges")
	a := b.CreateVariable("a")
	c := b.CreateVariable("c")
	require.NoError(t, a.Assign(b.EmitUndefined("a")))
	require.NoError(t, c.Assign(b.EmitUndefined("c")))
	ed := memedit.NewMemEditor("abc abc")
	a.SetDefRange(memedit.NewRangeFromOffsets(ed, 0, 3))
	c.SetDefRange(memedit.NewRangeFromOffsets(ed, 0, 3))
	original := memedit.NewRangeFromOffsets(ed, 0, 3)
	a.SetDefRange(original)
	original.GetEnd().SetColumn(2)
	require.Equal(t, 4, a.DefRange.GetEnd().GetColumn(), "shared positions must own the parser's mutable input")
	a.AddRange(memedit.NewRangeFromOffsets(ed, 4, 7), true)
	a.AddRange(memedit.NewRangeFromOffsets(ed, 4, 7), true)
	c.AddRange(memedit.NewRangeFromOffsets(ed, 4, 7), true)
	uses := func(v *Variable) (ret []*memedit.Range) {
		v.ForEachUseRange(func(r *memedit.Range) { ret = append(ret, r) })
		return
	}
	require.Len(t, uses(a), 1, "equal source positions must not be saved twice")
	require.Same(t, a.DefRange, c.DefRange, "variables should share equal definition ranges")
	require.Same(t, uses(a)[0], uses(c)[0], "variables should share equal use ranges")
	other := memedit.NewMemEditor("abc abc")
	a.AddRange(memedit.NewRangeFromOffsets(other, 4, 7), true)
	require.Len(t, uses(a), 2, "different editors must keep distinct locations")
	a.AddRange(nil, true)
	require.Len(t, uses(a), 2)
	b.Finish()
}

func TestVariableRangeConcurrentSharing(t *testing.T) {
	prog := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, prog)()
	b := prog.GetAndCreateFunctionBuilder("", "range-concurrency")
	v := b.CreateVariable("v")
	require.NoError(t, v.Assign(b.EmitUndefined("v")))
	v.skipPersistOffsets = true
	ed := memedit.NewMemEditor("01234567890123456789")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				v.AddRange(memedit.NewRangeFromOffsets(ed, j, j+1), true)
				v.ForEachUseRange(func(r *memedit.Range) { _ = r.GetText() })
			}
		}()
	}
	wg.Wait()
	count := 0
	v.ForEachUseRange(func(*memedit.Range) { count++ })
	require.Equal(t, 20, count)
	b.Finish()
}
