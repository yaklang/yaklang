package ssaapi

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// GuardPredicateValue is a lightweight ValueOperator carrier for structured guard predicates.
// It is returned by the <cfgGuards> native call.
//
// This is stage-2: it is intended to be machine-readable (stable fields), with optional
// human-readable Text for evidence.
type GuardPredicateValue struct {
	prog *Program

	FuncID int64

	// GuardBlockID is the basic block containing the branch/guard.
	GuardBlockID int64

	// SinkBlockID is the basic block for which the guard is considered relevant.
	SinkBlockID int64

	// CondInstID/CondValueID are best-effort IDs for locating the predicate in SSA/IR.
	CondInstID  int64
	CondValueID int64

	// Polarity describes the required truth value to reach the sink path.
	// Example: if (cond) return; sink => Polarity=false (need !cond).
	Polarity bool

	// Kind is a short classifier, e.g. "earlyReturn".
	Kind string

	// Text is optional evidence for humans; do not rely on it for matching.
	Text string

	anchorBits *utils.BitVector
}

var _ sfvm.ValueOperator = (*GuardPredicateValue)(nil)

func (g *GuardPredicateValue) String() string {
	if g == nil {
		return ""
	}
	text := g.Text
	if text == "" {
		if g.CondValueID > 0 {
			text = fmt.Sprintf("cond@%d", g.CondValueID)
		} else if g.CondInstID > 0 {
			text = fmt.Sprintf("condInst@%d", g.CondInstID)
		} else {
			text = "cond"
		}
	}
	return fmt.Sprintf("guard(kind=%s, func=%d, guardBlock=%d, sinkBlock=%d, polarity=%v, %s)",
		g.Kind, g.FuncID, g.GuardBlockID, g.SinkBlockID, g.Polarity, text,
	)
}

func (g *GuardPredicateValue) Hash() (string, bool) {
	if g == nil {
		return "", false
	}
	return fmt.Sprintf("guard:%d:%d:%d:%d:%d:%v:%s",
		g.FuncID, g.GuardBlockID, g.SinkBlockID, g.CondInstID, g.CondValueID, g.Polarity, g.Kind,
	), true
}

func (g *GuardPredicateValue) IsMap() bool  { return false }
func (g *GuardPredicateValue) IsList() bool { return false }

func (g *GuardPredicateValue) IsEmpty() bool {
	return g == nil || g.prog == nil || g.FuncID <= 0 || g.GuardBlockID <= 0 || g.SinkBlockID <= 0
}

func (g *GuardPredicateValue) ShouldUseConditionCandidate() bool { return false }

func (g *GuardPredicateValue) GetOpcode() string         { return "" }
func (g *GuardPredicateValue) GetBinaryOperator() string { return "" }
func (g *GuardPredicateValue) GetUnaryOperator() string  { return "" }

func (g *GuardPredicateValue) ExactMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}
func (g *GuardPredicateValue) GlobMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}
func (g *GuardPredicateValue) RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, sfvm.Values, error) {
	return false, sfvm.NewEmptyValues(), nil
}

func (g *GuardPredicateValue) GetCalled() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) GetCallActualParams(int, bool) (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) GetFields() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }

func (g *GuardPredicateValue) GetSyntaxFlowUse() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) GetSyntaxFlowDef() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) GetSyntaxFlowTopDef(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}
func (g *GuardPredicateValue) GetSyntaxFlowBottomUse(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (g *GuardPredicateValue) NewConst(v any, ranges ...*memedit.Range) sfvm.ValueOperator {
	if g == nil || g.prog == nil {
		return nil
	}
	return g.prog.NewConst(v, ranges...)
}

func (g *GuardPredicateValue) ListIndex(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("guard predicate is not list")
}

func (g *GuardPredicateValue) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	// no-op: guard predicate is a synthetic carrier value
	return nil
}

func (g *GuardPredicateValue) FileFilter(string, string, map[string]string, []string) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (g *GuardPredicateValue) CompareString(*sfvm.StringComparator) (sfvm.Values, []bool) { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) CompareOpcode(*sfvm.OpcodeComparator) (sfvm.Values, []bool) { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) CompareConst(*sfvm.ConstComparator) bool                     { return false }

func (g *GuardPredicateValue) GetAnchorBitVector() *utils.BitVector {
	if g == nil || g.anchorBits == nil {
		return nil
	}
	return g.anchorBits
}

func (g *GuardPredicateValue) SetAnchorBitVector(bits *utils.BitVector) {
	if g == nil {
		return
	}
	if bits == nil {
		g.anchorBits = nil
		return
	}
	g.anchorBits = bits.Clone()
}

