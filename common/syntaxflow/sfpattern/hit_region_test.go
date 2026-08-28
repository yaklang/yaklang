package sfpattern_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func hit(path string, start, end int) *sfvm.SimpleValue {
	return sfvm.NewSimpleValue("x", path, start, end)
}

func TestFilterContained(t *testing.T) {
	// ctx region [10, 40) in a.txt
	ctx := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 10, 40)})

	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 12, 20), // inside
		hit("a.txt", 5, 15),  // overlaps but not contained
		hit("a.txt", 30, 50), // overlaps but not contained
		hit("a.txt", 10, 40), // exact boundary — contained (inclusive)
		hit("b.txt", 12, 20), // same range, different file — not contained
	})
	res := sfpattern.FilterContained(target, ctx)
	require.Equal(t, 2, sfvm.ValuesLen(res))
	got := map[int]bool{}
	_ = res.Recursive(func(v sfvm.ValueOperator) error {
		sv := v.(*sfvm.SimpleValue)
		got[sv.Start()] = true
		return nil
	})
	require.True(t, got[12])
	require.True(t, got[10])
}

func TestFilterContained_NoContext(t *testing.T) {
	target := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 1, 2)})
	res := sfpattern.FilterContained(target, sfvm.NewEmptyValues())
	require.True(t, res.IsEmpty())
}

func TestFilterContained_NonHitPassThrough(t *testing.T) {
	ctx := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 0, 100)})
	constVal := sfvm.NewSimpleConst("const")
	target := sfvm.NewValues([]sfvm.ValueOperator{constVal, hit("a.txt", 5, 6)})
	res := sfpattern.FilterContained(target, ctx)
	require.Equal(t, 2, sfvm.ValuesLen(res))
}

func TestFilterNotContained(t *testing.T) {
	ctx := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 10, 40)})
	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 12, 20), // inside — dropped
		hit("a.txt", 5, 15),  // overlaps only — kept
		hit("b.txt", 12, 20), // other file — kept
	})
	res := sfpattern.FilterNotContained(target, ctx)
	require.Equal(t, 2, sfvm.ValuesLen(res))
}

func TestFilterNotContained_NoContext(t *testing.T) {
	target := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 1, 2)})
	res := sfpattern.FilterNotContained(target, sfvm.NewEmptyValues())
	require.Equal(t, 1, sfvm.ValuesLen(res))
}

func TestFilterOverlap(t *testing.T) {
	// AND semantics: target hit must overlap at least one hit of EVERY other set.
	other1 := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 10, 40)})
	other2 := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 30, 60)})

	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 12, 20), // overlaps other1 only — fails AND
		hit("a.txt", 35, 45), // overlaps both — kept
		hit("a.txt", 50, 55), // overlaps other2 only — fails AND
		hit("b.txt", 35, 45), // other file — fails
	})
	res := sfpattern.FilterOverlap(target, other1, other2)
	require.Equal(t, 1, sfvm.ValuesLen(res))
	sv, _ := res.First()
	require.Equal(t, 35, sv.(*sfvm.SimpleValue).Start())
}

func TestFilterOverlap_NoOthers(t *testing.T) {
	target := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 1, 2)})
	res := sfpattern.FilterOverlap(target)
	require.Equal(t, 1, sfvm.ValuesLen(res))
}

func TestFilterOverlap_ZeroLengthHit(t *testing.T) {
	// zero-length hit at p overlaps region [s,e) when s < p < e
	other := sfvm.NewValues([]sfvm.ValueOperator{hit("a.txt", 10, 40)})
	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 20, 20), // inside region — kept
		hit("a.txt", 5, 5),   // outside — dropped
	})
	res := sfpattern.FilterOverlap(target, other)
	require.Equal(t, 1, sfvm.ValuesLen(res))
	sv, _ := res.First()
	require.Equal(t, 20, sv.(*sfvm.SimpleValue).Start())
}
