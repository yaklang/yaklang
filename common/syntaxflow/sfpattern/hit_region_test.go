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

// Context regions can nest: a double-quoted string lives inside a
// single-quoted one in PHP, comments can appear inside strings, and so on.
// Only the region that starts last before the target used to be considered,
// so a target covered by an earlier, longer region leaked through.
func TestFilterNotContained_NestedContextRegions(t *testing.T) {
	ctx := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 10, 100), // long single-quoted string containing the rest
		hit("a.txt", 30, 40),  // short double-quoted string nested inside
	})
	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 60, 70), // outside the nested one, inside the long one
		hit("a.txt", 32, 38), // inside both
		hit("a.txt", 200, 210),
	})
	res := sfpattern.FilterNotContained(target, ctx)
	require.Equal(t, 1, sfvm.ValuesLen(res))
}

// Same nesting hazard for the overlap filter: a long region starting before a
// shorter nested one must still match targets that only the long one covers.
func TestFilterOverlap_NestedContextRegions(t *testing.T) {
	other := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 10, 100),
		hit("a.txt", 30, 40),
	})
	target := sfvm.NewValues([]sfvm.ValueOperator{
		hit("a.txt", 60, 70), // only the long region overlaps
		hit("a.txt", 300, 310),
	})
	res := sfpattern.FilterOverlap(target, other)
	require.Equal(t, 1, sfvm.ValuesLen(res))
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
