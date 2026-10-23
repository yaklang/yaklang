package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

// The fixture supplies already-resolved members, without compiling or reading DB.
type searchWrapperObject struct {
	ssa.Value
	pairs []ssa.MemberPair
}

func (v *searchWrapperObject) Self() ssa.Instruction            { return v }
func (v *searchWrapperObject) GetMemberPairs() []ssa.MemberPair { return v.pairs }
func (v *searchWrapperObject) IsObject() bool                   { return true }

func searchWrapperFixture() (*Program, *Value) {
	p := &Program{id: atomic.NewInt64(0)}
	a, b := ssa.NewConst(10), ssa.NewConst(20)
	a.SetId(2)
	b.SetId(3)
	key := ssa.NewConst("needle")
	key.SetName("key_alias")
	o := &searchWrapperObject{Value: ssa.NewConst("object"), pairs: []ssa.MemberPair{
		{Key: key, Member: a}, {Key: ssa.NewConst("other"), Member: b},
	}}
	v, err := p.NewValue(o)
	if err != nil {
		panic(err)
	}
	return p, v
}

func TestMemberSearchPreservesMatchingAndIndependentState(t *testing.T) {
	_, v := searchWrapperFixture()
	for _, text := range []string{"key_alias", "needle", `"needle"`} {
		got := SearchWithValue(v, ssadb.KeyMatch, func(s string) bool { return s == text })
		require.Len(t, got, 1)
		require.Equal(t, int64(2), got[0].GetId())
		require.Len(t, got[0].Predecessors, 1)
		require.Same(t, v, got[0].Predecessors[0].Node)
	}
	a := SearchWithValue(v, ssadb.KeyMatch, func(s string) bool { return s == "needle" })
	b := SearchWithValue(v, ssadb.KeyMatch, func(s string) bool { return s == "needle" })
	require.NotSame(t, a[0], b[0])
	require.NotEqual(t, a[0].GetUID(), b[0].GetUID())
	a[0].Predecessors = nil
	require.Len(t, b[0].Predecessors, 1)
}

func TestUnmatchedMemberSearchDoesNotCreateValueWrappers(t *testing.T) {
	p, v := searchWrapperFixture()
	before := p.id.Load()
	require.Empty(t, SearchWithValue(v, ssadb.KeyMatch, func(string) bool { return false }))
	require.Equal(t, before, p.id.Load(), "unmatched keys need no analysis identity")
}

func TestMetadataSearchModesKeepMatching(t *testing.T) {
	p := &Program{id: atomic.NewInt64(0)}
	for _, input := range []any{"needle", 42, true, nil} {
		inst := ssa.NewConst(input)
		inst.SetName("alias")
		v, err := p.NewValue(inst)
		require.NoError(t, err)
		for _, mode := range []ssadb.MatchMode{ssadb.NameMatch, ssadb.ConstType} {
			before := p.id.Load()
			got := SearchWithValue(v, mode, func(text string) bool { return text == "alias" })
			require.Len(t, got, 1)
			require.Same(t, v, got[0])
			require.Equal(t, before, p.id.Load())
			require.Empty(t, SearchWithValue(v, mode, func(string) bool { return false }))
		}
	}
}

func BenchmarkUnmatchedMemberSearch(b *testing.B) {
	_, v := searchWrapperFixture()
	compare := func(string) bool { return false }
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		SearchWithValue(v, ssadb.KeyMatch, compare)
	}
}
