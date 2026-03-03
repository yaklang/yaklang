package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type bitVectorValue struct {
	*ValueList
	name       string
	sourceBits *utils.BitVector
}

func newBitVectorValue(name string) *bitVectorValue {
	return &bitVectorValue{
		ValueList: &ValueList{},
		name:      name,
	}
}

func (v *bitVectorValue) String() string { return v.name }

func (v *bitVectorValue) IsEmpty() bool { return v == nil }

func (v *bitVectorValue) Recursive(f func(ValueOperator) error) error {
	if v == nil {
		return nil
	}
	return f(v)
}

func (v *bitVectorValue) Merge(...ValueOperator) (ValueOperator, error) {
	return nil, utils.Error("merge unsupported")
}

func (v *bitVectorValue) GetSourceBitVector() *utils.BitVector {
	if v == nil || v.sourceBits == nil {
		return nil
	}
	return v.sourceBits
}

func (v *bitVectorValue) SetSourceBitVector(bits *utils.BitVector) {
	if v == nil {
		return
	}
	if bits == nil {
		v.sourceBits = nil
		return
	}
	v.sourceBits = bits.Clone()
}

func TestNormalizeConditionAgainstSource_UsesBitVectorForDuplicateSource(t *testing.T) {
	shared := newBitVectorValue("shared")
	source := &ValueList{Values: []ValueOperator{shared, shared}}
	result := &ValueList{Values: []ValueOperator{shared}}

	mask, err := normalizeConditionAgainstSource(source, result, []bool{true})
	require.NoError(t, err)
	require.Equal(t, []bool{true, true}, mask)
}

func TestBuildFilterMask_UsesBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := &ValueList{Values: []ValueOperator{a, b, c}}

	condVal := newBitVectorValue("cond")
	condBits := utils.NewBitVector()
	condBits.Set(0)
	condBits.Set(2)
	condVal.SetSourceBitVector(condBits)

	mask, err := buildFilterMask(source, &ValueList{Values: []ValueOperator{condVal}})
	require.NoError(t, err)
	require.Equal(t, []bool{true, false, true}, mask)
}

func TestBuildFilterMask_DerivesMaskFromSourceValueBitVector(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := &ValueList{Values: []ValueOperator{a, b, c}}

	// Simulate `filter` condition output directly reusing one source element.
	mask, err := buildFilterMask(source, &ValueList{Values: []ValueOperator{b}})
	require.NoError(t, err)
	require.Equal(t, []bool{false, true, false}, mask)
}

func TestFilterValueByMask(t *testing.T) {
	a := newBitVectorValue("a")
	b := newBitVectorValue("b")
	c := newBitVectorValue("c")
	source := &ValueList{Values: []ValueOperator{a, b, c}}

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
	source := &ValueList{Values: []ValueOperator{a, b}}

	_, err := filterValueByMask(source, []bool{true})
	require.Error(t, err)
}
