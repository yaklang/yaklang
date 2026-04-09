package ssaapi

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// CfgCtxValue is a lightweight ValueOperator wrapper used as the return carrier
// for the <getCfg> native call.
//
// It intentionally does NOT participate in normal graph navigation (def-use,
// called/fields, etc.). It only exists so subsequent native calls can take a
// stable "program point" input (function/block/inst) and run CFG algorithms.
type CfgCtxValue struct {
	prog *Program

	FuncID  int64
	BlockID int64
	InstID  int64

	anchorBits *utils.BitVector
}

var _ sfvm.ValueOperator = (*CfgCtxValue)(nil)

func (c *CfgCtxValue) String() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("cfg(func=%d, block=%d, inst=%d)", c.FuncID, c.BlockID, c.InstID)
}

func (c *CfgCtxValue) IsMap() bool  { return false }
func (c *CfgCtxValue) IsList() bool { return false }
func (c *CfgCtxValue) IsEmpty() bool {
	return c == nil || c.prog == nil || c.FuncID <= 0 || c.BlockID <= 0
}

func (c *CfgCtxValue) ShouldUseConditionCandidate() bool { return false }

func (c *CfgCtxValue) GetOpcode() string         { return "" }
func (c *CfgCtxValue) GetBinaryOperator() string { return "" }
func (c *CfgCtxValue) GetUnaryOperator() string  { return "" }

func (c *CfgCtxValue) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}
func (c *CfgCtxValue) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}
func (c *CfgCtxValue) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}

func (c *CfgCtxValue) GetCalled() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }
func (c *CfgCtxValue) GetCallActualParams(int, bool) (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (c *CfgCtxValue) GetFields() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }

func (c *CfgCtxValue) GetSyntaxFlowUse() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (c *CfgCtxValue) GetSyntaxFlowDef() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }

func (c *CfgCtxValue) GetSyntaxFlowTopDef(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}
func (c *CfgCtxValue) GetSyntaxFlowBottomUse(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (c *CfgCtxValue) ListIndex(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("cfg ctx is not list")
}

func (c *CfgCtxValue) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	// no-op: cfg ctx is a synthetic carrier value
	return nil
}

func (c *CfgCtxValue) FileFilter(string, string, map[string]string, []string) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (c *CfgCtxValue) CompareString(*sfvm.StringComparator) (sfvm.Values, []bool) { return sfvm.NewEmptyValues(), nil }
func (c *CfgCtxValue) CompareOpcode(*sfvm.OpcodeComparator) (sfvm.Values, []bool) { return sfvm.NewEmptyValues(), nil }
func (c *CfgCtxValue) CompareConst(*sfvm.ConstComparator) bool                     { return false }

func (c *CfgCtxValue) NewConst(v any, ranges ...*memedit.Range) sfvm.ValueOperator {
	if c == nil || c.prog == nil {
		return nil
	}
	return c.prog.NewConst(v, ranges...)
}

func (c *CfgCtxValue) GetAnchorBitVector() *utils.BitVector {
	if c == nil || c.anchorBits == nil {
		return nil
	}
	return c.anchorBits
}

func (c *CfgCtxValue) SetAnchorBitVector(bits *utils.BitVector) {
	if c == nil {
		return
	}
	if bits == nil {
		c.anchorBits = nil
		return
	}
	c.anchorBits = bits.Clone()
}

