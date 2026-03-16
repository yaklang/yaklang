package sfvm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type stubValueOperator struct{}

func (stubValueOperator) String() string { return "" }
func (stubValueOperator) IsMap() bool    { return false }
func (stubValueOperator) IsList() bool   { return false }
func (stubValueOperator) IsEmpty() bool  { return false }
func (stubValueOperator) ShouldUseConditionCandidate() bool {
	return false
}
func (stubValueOperator) GetOpcode() string         { return "" }
func (stubValueOperator) GetBinaryOperator() string { return "" }
func (stubValueOperator) GetUnaryOperator() string  { return "" }
func (stubValueOperator) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (stubValueOperator) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (stubValueOperator) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error) {
	return false, nil, nil
}
func (stubValueOperator) GetCalled() (Values, error)                    { return nil, nil }
func (stubValueOperator) GetCallActualParams(int, bool) (Values, error) { return nil, nil }
func (stubValueOperator) GetFields() (Values, error)                    { return nil, nil }
func (stubValueOperator) GetSyntaxFlowUse() (Values, error)             { return nil, nil }
func (stubValueOperator) GetSyntaxFlowDef() (Values, error)             { return nil, nil }
func (stubValueOperator) GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (stubValueOperator) GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error) {
	return nil, nil
}
func (stubValueOperator) ListIndex(int) (ValueOperator, error) {
	return nil, utils.Error("unsupported")
}
func (stubValueOperator) AppendPredecessor(ValueOperator, ...AnalysisContextOption) error {
	return nil
}
func (stubValueOperator) FileFilter(string, string, map[string]string, []string) (Values, error) {
	return nil, nil
}
func (stubValueOperator) CompareString(*StringComparator) (Values, []bool) { return nil, nil }
func (stubValueOperator) CompareOpcode(*OpcodeComparator) (Values, []bool) { return nil, nil }
func (stubValueOperator) CompareConst(*ConstComparator) bool               { return false }
func (stubValueOperator) NewConst(any, ...*memedit.Range) ValueOperator    { return nil }
func (stubValueOperator) GetAnchorBitVector() *utils.BitVector             { return nil }
func (stubValueOperator) SetAnchorBitVector(*utils.BitVector)              {}

type bitVectorValue struct {
	stubValueOperator
	name       string
	anchorBits *utils.BitVector
}

func newBitVectorValue(name string) *bitVectorValue {
	return &bitVectorValue{name: name}
}

func (v *bitVectorValue) String() string { return v.name }
func (v *bitVectorValue) IsEmpty() bool  { return v == nil }
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

	anchorRestore := assignLocalAnchorBitVector(source, 0)
	defer restoreAnchorBitVector(anchorRestore)

	scope := anchorScopeState{
		anchorWidth: len(source),
	}
	mask, err := normalizeConditionAgainstSource(scope, result, []bool{true})
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
	scope := anchorScopeState{
		anchorWidth: len(source),
	}
	mask, err := normalizeConditionAgainstSource(scope, nil, []bool{true, false})
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

	scope := anchorScopeState{
		anchorWidth: len(source),
	}
	mask, err := buildFilterMask(scope, NewValues([]ValueOperator{condVal}))
	require.NoError(t, err)
	require.Equal(t, []bool{true, false, true}, mask)
}

func TestBuildFilterMask_DerivesMaskFromSourceValueBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	anchorRestore := assignLocalAnchorBitVector(source, 0)
	defer restoreAnchorBitVector(anchorRestore)

	scope := anchorScopeState{
		anchorWidth: len(source),
	}
	mask, err := buildFilterMask(scope, NewValues([]ValueOperator{b}))
	require.NoError(t, err)
	require.Equal(t, []bool{false, true, false}, mask)
}

func TestApplyCondition_Mask(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := NewValues([]ValueOperator{a, b, c})

	entry := newMaskCondition([]bool{true, false, true})
	filtered, err := entry.Apply(source)
	require.NoError(t, err)
	require.Equal(t, 2, ValuesLen(filtered))

	first, err := filtered.ListIndex(0)
	require.NoError(t, err)
	second, err := filtered.ListIndex(1)
	require.NoError(t, err)
	require.Equal(t, "a", first.String())
	require.Equal(t, "c", second.String())
}

func TestApplyCondition_EmptyValueReturnsEmpty(t *testing.T) {
	entry := newMaskCondition([]bool{true})
	filtered, err := entry.Apply(NewEmptyValues())
	require.NoError(t, err)
	require.True(t, filtered.IsEmpty())
}

func TestApplyCondition_MaskLengthMismatch(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	source := NewValues([]ValueOperator{a, b})

	entry := newMaskCondition([]bool{true})
	_, err := entry.Apply(source)
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

	scope := anchorScopeState{
		anchorWidth: len(source),
	}
	mask, err := normalizeConditionAgainstSource(scope, NewValues([]ValueOperator{cond}), nil)
	require.NoError(t, err)
	require.Equal(t, []bool{true, false, true}, mask)
}
