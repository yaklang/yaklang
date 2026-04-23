package ssaapi

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// GuardKind* are stable strings for GuardPredicateValue.Kind / <cfgGuards> output.
const (
	GuardKindEarlyReturn   = "earlyReturn"
	GuardKindEarlyPanic    = "earlyPanic"
	GuardKindEarlyBreak    = "earlyBreak"
	GuardKindEarlyContinue = "earlyContinue"
	// GuardKindNone is emitted when cfgGuards finds no qualifying branch guard for the sink.
	GuardKindNone = "none"
)

// GuardFieldValue is a synthetic member carrier used by GuardPredicateValue.GetFields().
//
// It matches by member-key (ssadb.KeyMatch) on FieldKey, and delegates const/string
// comparisons to the underlying ValueOperator.
type GuardFieldValue struct {
	prog     *Program
	FieldKey string
	Val      sfvm.ValueOperator

	anchorBits *utils.BitVector
}

var _ sfvm.ValueOperator = (*GuardFieldValue)(nil)
var _ ssa.GetIdIF = (*GuardFieldValue)(nil)

func (f *GuardFieldValue) GetId() int64 {
	if f == nil {
		return 0
	}
	if id, ok := f.Val.(ssa.GetIdIF); ok {
		return id.GetId()
	}
	return 0
}

func (f *GuardFieldValue) String() string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("guardField(%s=%v)", f.FieldKey, f.Val)
}

func (f *GuardFieldValue) IsMap() bool  { return false }
func (f *GuardFieldValue) IsList() bool { return false }
func (f *GuardFieldValue) IsEmpty() bool {
	return f == nil || f.prog == nil || f.FieldKey == ""
}

func (f *GuardFieldValue) ShouldUseConditionCandidate() bool { return false }
func (f *GuardFieldValue) GetOpcode() string                 { return "" }
func (f *GuardFieldValue) GetBinaryOperator() string         { return "" }
func (f *GuardFieldValue) GetUnaryOperator() string          { return "" }

func (f *GuardFieldValue) ExactMatch(_ context.Context, mod ssadb.MatchMode, s string) (bool, sfvm.Values, error) {
	// Field selection uses KeyMatch via GetFields().
	if f == nil {
		return false, sfvm.NewEmptyValues(), nil
	}
	if mod == ssadb.KeyMatch && s == f.FieldKey {
		return true, sfvm.ValuesOf(f), nil
	}
	return false, sfvm.NewEmptyValues(), nil
}
func (f *GuardFieldValue) GlobMatch(_ context.Context, mod ssadb.MatchMode, g string) (bool, sfvm.Values, error) {
	if f == nil {
		return false, sfvm.NewEmptyValues(), nil
	}
	if mod == ssadb.KeyMatch && utils.MatchAnyOfGlob(f.FieldKey, g) {
		return true, sfvm.ValuesOf(f), nil
	}
	return false, sfvm.NewEmptyValues(), nil
}
func (f *GuardFieldValue) RegexpMatch(_ context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	if f == nil {
		return false, sfvm.NewEmptyValues(), nil
	}
	if mod == ssadb.KeyMatch && utils.MatchAnyOfRegexp(f.FieldKey, re) {
		return true, sfvm.ValuesOf(f), nil
	}
	return false, sfvm.NewEmptyValues(), nil
}

func (f *GuardFieldValue) GetCalled() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }
func (f *GuardFieldValue) GetCallActualParams(int, bool) (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }

func (f *GuardFieldValue) GetFields() (sfvm.Values, error) {
	// Member lookup happens on this node itself.
	if f == nil || f.prog == nil || f.FieldKey == "" {
		return sfvm.NewEmptyValues(), nil
	}
	// Return a single const node whose key is FieldKey. The VM uses KeyMatch to find it.
	// We represent it as a *Value const so it has an ID and can be visited/deduped.
	// The actual value is stored in Val and used for comparisons.
	return sfvm.NewValues([]sfvm.ValueOperator{f}), nil
}

func (f *GuardFieldValue) GetSyntaxFlowUse() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (f *GuardFieldValue) GetSyntaxFlowDef() (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }
func (f *GuardFieldValue) GetSyntaxFlowTopDef(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}
func (f *GuardFieldValue) GetSyntaxFlowBottomUse(*sfvm.SFFrameResult, *sfvm.Config, ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (f *GuardFieldValue) ListIndex(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("guard field is not list")
}

func (f *GuardFieldValue) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error { return nil }
func (f *GuardFieldValue) FileFilter(string, string, map[string]string, []string) (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (f *GuardFieldValue) CompareString(c *sfvm.StringComparator) (sfvm.Values, []bool) {
	if f == nil || f.Val == nil {
		return sfvm.NewEmptyValues(), nil
	}
	return f.Val.CompareString(c)
}
func (f *GuardFieldValue) CompareOpcode(c *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
	if f == nil || f.Val == nil {
		return sfvm.NewEmptyValues(), nil
	}
	return f.Val.CompareOpcode(c)
}
func (f *GuardFieldValue) CompareConst(c *sfvm.ConstComparator) bool {
	if f == nil || f.Val == nil {
		return false
	}
	return f.Val.CompareConst(c)
}

func (f *GuardFieldValue) NewConst(v any, ranges ...*memedit.Range) sfvm.ValueOperator {
	if f == nil || f.prog == nil {
		return nil
	}
	return f.prog.NewConstValue(v, ranges...)
}

func (f *GuardFieldValue) GetAnchorBitVector() *utils.BitVector {
	if f == nil || f.anchorBits == nil {
		return nil
	}
	return f.anchorBits
}
func (f *GuardFieldValue) SetAnchorBitVector(bits *utils.BitVector) {
	if f == nil {
		return
	}
	if bits == nil {
		f.anchorBits = nil
		return
	}
	f.anchorBits = bits.Clone()
}

// GuardPredicateValue is a lightweight ValueOperator carrier for structured guard predicates.
// It is returned by the <cfgGuards> native call.
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
	Polarity bool

	// Kind is a short classifier; use GuardKind* constants (earlyReturn / earlyPanic /
	// earlyBreak / earlyContinue).
	Kind string

	// Text is optional evidence for humans; do not rely on it for matching.
	Text string

	anchorBits *utils.BitVector
}

var _ sfvm.ValueOperator = (*GuardPredicateValue)(nil)
var _ ssa.GetIdIF = (*GuardPredicateValue)(nil)

// guardPredicateHashToID maps the canonical Hash() string to a positive int64 for SF traversal
// (visited sets / GetIdIF). Reuses Hash() so GetId stays aligned with dedup keys.
func guardPredicateHashToID(key string) int64 {
	if key == "" {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	u := h.Sum64()
	// Positive int64 for stable map keys; FNV output is folded into 63 bits.
	id := int64(u & 0x7fffffffffffffff)
	if id == 0 {
		return 1
	}
	return id
}

func (g *GuardPredicateValue) GetId() int64 {
	if g == nil {
		return 0
	}
	key, ok := g.Hash()
	if !ok {
		return 0
	}
	return guardPredicateHashToID(key)
}

func (g *GuardPredicateValue) String() string {
	if g == nil {
		return ""
	}
	// cfgGuard: CFG-oriented guard line; guards are cfg natives specialized on branch→sink evidence.
	if g.Kind == GuardKindNone && g.CondInstID == 0 && g.CondValueID == 0 && g.GuardBlockID == g.SinkBlockID {
		return fmt.Sprintf("cfgGuard(kind=%s, synthetic, fn=%d, blk=%d)", g.Kind, g.FuncID, g.SinkBlockID)
	}
	text := g.Text
	if text == "" {
		if g.CondValueID > 0 {
			text = fmt.Sprintf("condVal=%d", g.CondValueID)
		} else if g.CondInstID > 0 {
			text = fmt.Sprintf("condInst=%d", g.CondInstID)
		} else {
			text = "cond=?"
		}
	}
	return fmt.Sprintf("cfgGuard(kind=%s, fn=%d, guardBlk=%d, sinkBlk=%d, polarity=%v, %s)",
		g.Kind, g.FuncID, g.GuardBlockID, g.SinkBlockID, g.Polarity, text,
	)
}

// SFVMVerboseResultString is used by sfvm SyntaxFlow result listing to print the full cfgGuard line
// without ShrinkString (see sfvm/showValueMap).
func (g *GuardPredicateValue) SFVMVerboseResultString() string {
	if g == nil {
		return ""
	}
	return g.String()
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

func (g *GuardPredicateValue) fieldConstByKey(key string) (sfvm.ValueOperator, bool) {
	if g == nil || g.prog == nil {
		return nil, false
	}
	switch key {
	case "kind":
		return g.prog.NewConstValue(g.Kind), true
	case "polarity":
		return g.prog.NewConstValue(g.Polarity), true
	case "func_id":
		return g.prog.NewConstValue(g.FuncID), true
	case "guard_block_id":
		return g.prog.NewConstValue(g.GuardBlockID), true
	case "sink_block_id":
		return g.prog.NewConstValue(g.SinkBlockID), true
	case "cond_inst_id":
		return g.prog.NewConstValue(g.CondInstID), true
	case "cond_value_id":
		return g.prog.NewConstValue(g.CondValueID), true
	case "text":
		if g.Text == "" {
			return nil, false
		}
		return g.prog.NewConstValue(g.Text), true
	default:
		return nil, false
	}
}

func (g *GuardPredicateValue) ExactMatch(_ context.Context, mod ssadb.MatchMode, s string) (bool, sfvm.Values, error) {
	if mod != ssadb.KeyMatch {
		return false, sfvm.NewEmptyValues(), nil
	}
	if v, ok := g.fieldConstByKey(s); ok && v != nil && !v.IsEmpty() {
		sfvm.MergeAnchor(g, v)
		return true, sfvm.ValuesOf(v), nil
	}
	return false, sfvm.NewEmptyValues(), nil
}

func (g *GuardPredicateValue) GlobMatch(_ context.Context, mod ssadb.MatchMode, pattern string) (bool, sfvm.Values, error) {
	if mod != ssadb.KeyMatch {
		return false, sfvm.NewEmptyValues(), nil
	}
	keys := []string{"kind", "polarity", "func_id", "guard_block_id", "sink_block_id", "cond_inst_id", "cond_value_id", "text"}
	var out []sfvm.ValueOperator
	for _, k := range keys {
		if !utils.MatchAnyOfGlob(k, pattern) {
			continue
		}
		if v, ok := g.fieldConstByKey(k); ok && v != nil && !v.IsEmpty() {
			sfvm.MergeAnchor(g, v)
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	return true, sfvm.NewValues(out), nil
}

func (g *GuardPredicateValue) RegexpMatch(_ context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	if mod != ssadb.KeyMatch {
		return false, sfvm.NewEmptyValues(), nil
	}
	keys := []string{"kind", "polarity", "func_id", "guard_block_id", "sink_block_id", "cond_inst_id", "cond_value_id", "text"}
	var out []sfvm.ValueOperator
	for _, k := range keys {
		if !utils.MatchAnyOfRegexp(k, re) {
			continue
		}
		if v, ok := g.fieldConstByKey(k); ok && v != nil && !v.IsEmpty() {
			sfvm.MergeAnchor(g, v)
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	return true, sfvm.NewValues(out), nil
}

func (g *GuardPredicateValue) GetCalled() (sfvm.Values, error)                    { return sfvm.NewEmptyValues(), nil }
func (g *GuardPredicateValue) GetCallActualParams(int, bool) (sfvm.Values, error) { return sfvm.NewEmptyValues(), nil }

func (g *GuardPredicateValue) GetFields() (sfvm.Values, error) {
	if g == nil || g.prog == nil {
		return sfvm.NewEmptyValues(), nil
	}
	cv := func(v any) sfvm.ValueOperator { return g.prog.NewConstValue(v) }
	fields := []sfvm.ValueOperator{
		&GuardFieldValue{prog: g.prog, FieldKey: "kind", Val: cv(g.Kind), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "polarity", Val: cv(g.Polarity), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "func_id", Val: cv(g.FuncID), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "guard_block_id", Val: cv(g.GuardBlockID), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "sink_block_id", Val: cv(g.SinkBlockID), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "cond_inst_id", Val: cv(g.CondInstID), anchorBits: g.anchorBits},
		&GuardFieldValue{prog: g.prog, FieldKey: "cond_value_id", Val: cv(g.CondValueID), anchorBits: g.anchorBits},
	}
	if g.Text != "" {
		fields = append(fields, &GuardFieldValue{prog: g.prog, FieldKey: "text", Val: cv(g.Text), anchorBits: g.anchorBits})
	}
	return sfvm.NewValues(fields), nil
}

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
	return g.prog.NewConstValue(v, ranges...)
}

func (g *GuardPredicateValue) ListIndex(int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("guard predicate is not list")
}

func (g *GuardPredicateValue) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error { return nil }
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

