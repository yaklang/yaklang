package sfvm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type bitVectorValue struct {
	name       string
	anchorBits *utils.BitVector
}

func newBitVectorValue(name string) *bitVectorValue {
	return &bitVectorValue{name: name}
}

func (v *bitVectorValue) String() string { return v.name }
func (v *bitVectorValue) IsMap() bool    { return false }
func (v *bitVectorValue) IsList() bool   { return false }
func (v *bitVectorValue) IsEmpty() bool  { return v == nil }
func (v *bitVectorValue) ShouldUseConditionCandidate() bool {
	return false
}
func (v *bitVectorValue) GetOpcode() string         { return "" }
func (v *bitVectorValue) GetBinaryOperator() string { return "" }
func (v *bitVectorValue) GetUnaryOperator() string  { return "" }
func (v *bitVectorValue) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *bitVectorValue) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *bitVectorValue) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (v *bitVectorValue) GetCalled() (Values, error) { return nil, nil }
func (v *bitVectorValue) GetCallActualParams(int, bool) (Values, error) {
	return nil, nil
}
func (v *bitVectorValue) GetFields() (Values, error)        { return nil, nil }
func (v *bitVectorValue) GetSyntaxFlowUse() (Values, error) { return nil, nil }
func (v *bitVectorValue) GetSyntaxFlowDef() (Values, error) { return nil, nil }
func (v *bitVectorValue) GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (v *bitVectorValue) GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (v *bitVectorValue) ListIndex(i int) (ValueOperator, error) {
	if i != 0 {
		return nil, utils.Error("index out of range")
	}
	return v, nil
}
func (v *bitVectorValue) AppendPredecessor(ValueOperator, ...AnalysisContextOption) error { return nil }
func (v *bitVectorValue) FileFilter(string, string, map[string]string, []string) (Values, error) {
	return nil, nil
}
func (v *bitVectorValue) CompareString(*StringComparator) (Values, []bool) {
	return ValuesOf(v), []bool{false}
}
func (v *bitVectorValue) CompareOpcode(*OpcodeComparator) (Values, []bool) {
	return ValuesOf(v), []bool{false}
}
func (v *bitVectorValue) CompareConst(*ConstComparator) bool { return false }
func (v *bitVectorValue) NewConst(any, ...*memedit.Range) ValueOperator {
	return v
}
func (v *bitVectorValue) GetAnchorBitVector() *utils.BitVector {
	if v == nil || v.anchorBits == nil {
		return nil
	}
	return v.anchorBits
}
func (v *bitVectorValue) SetAnchorBitVector(bits *utils.BitVector) {
	if v == nil {
		return
	}
	if bits == nil {
		v.anchorBits = nil
		return
	}
	v.anchorBits = bits.Clone()
}

func TestNormalizeConditionAgainstSource_UsesBitVectorForDuplicateSource(t *testing.T) {
	shared := newBitVectorValue("shared")
	source := NewValues([]ValueOperator{shared, shared})
	result := NewValues([]ValueOperator{shared})

	mask, err := normalizeConditionAgainstSource(source, result, []bool{true})
	require.NoError(t, err)
	require.Equal(t, []bool{true, true}, mask)
}

func TestNormalizeConditionAgainstSource_AlignedConditionShouldNotRewriteSourceBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	aBits := utils.NewBitVector()
	aBits.Set(3)
	a.SetAnchorBitVector(aBits)

	b := newBitVectorValue("b")
	bBits := utils.NewBitVector()
	bBits.Set(5)
	b.SetAnchorBitVector(bBits)

	source := NewValues([]ValueOperator{a, b})
	mask, err := normalizeConditionAgainstSource(source, nil, []bool{true, false})
	require.NoError(t, err)
	require.Equal(t, []bool{true, false}, mask)

	require.True(t, a.GetAnchorBitVector().Has(3))
	require.False(t, a.GetAnchorBitVector().Has(0))
	require.True(t, b.GetAnchorBitVector().Has(5))
	require.False(t, b.GetAnchorBitVector().Has(1))
}

func TestBuildFilterMask_UsesBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	condVal := newBitVectorValue("cond")
	condBits := utils.NewBitVector()
	condBits.Set(0)
	condBits.Set(2)
	condVal.SetAnchorBitVector(condBits)

	mask, err := buildFilterMask(source, NewValues([]ValueOperator{condVal}))
	require.NoError(t, err)
	require.Equal(t, []bool{true, false, true}, mask)
}

func TestBuildFilterMask_DerivesMaskFromSourceValueBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	mask, err := buildFilterMask(source, NewValues([]ValueOperator{b}))
	require.NoError(t, err)
	require.Equal(t, []bool{false, true, false}, mask)
}

func TestFilterValueByMask(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	filtered, err := filterValueByMask(source, []bool{true, false, true})
	require.NoError(t, err)
	require.Equal(t, 2, ValuesLen(filtered))

	first, err := filtered.ListIndex(0)
	require.NoError(t, err)
	second, err := filtered.ListIndex(1)
	require.NoError(t, err)
	require.Equal(t, "a", first.String())
	require.Equal(t, "c", second.String())
}

func TestFilterValueByMask_LengthMismatch(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	source := NewValues([]ValueOperator{a, b})

	_, err := filterValueByMask(source, []bool{true})
	require.Error(t, err)
}

func TestNormalizeConditionAgainstSource_DerivesMaskFromAnchorBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	cond := newBitVectorValue("cond")
	condBits := utils.NewBitVector()
	condBits.Set(0)
	condBits.Set(2)
	cond.SetAnchorBitVector(condBits)

	mask, err := normalizeConditionAgainstSource(source, NewValues([]ValueOperator{cond}), nil)
	require.NoError(t, err)
	require.Equal(t, []bool{true, false, true}, mask)
}
