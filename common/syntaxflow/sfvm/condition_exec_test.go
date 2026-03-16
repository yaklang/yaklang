package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

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
