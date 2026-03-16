package sfvm

import (
	"context"

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
