package sfvm

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"strings"

	"github.com/yaklang/yaklang/common/utils/omap"
)

func (y *SyntaxFlowVisitor) EmitExitStatement() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpExitStatement,
	})
}

func (y *SyntaxFlowVisitor) EmitEnterStatement() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpEnterStatement,
	})
}

func (y *SyntaxFlowVisitor) EmitNewRef(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpNewRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitUpdate(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpUpdateRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitOperator(i string) {
	switch i {
	case ">":
		y.codes = append(y.codes, &SFI{OpCode: OpGt})
	case ">=":
		y.codes = append(y.codes, &SFI{OpCode: OpGtEq})
	case "<":
		y.codes = append(y.codes, &SFI{OpCode: OpLt})
	case "<=":
		y.codes = append(y.codes, &SFI{OpCode: OpLtEq})
	case "==", "=":
		y.codes = append(y.codes, &SFI{OpCode: OpEq})
	case "!=":
		y.codes = append(y.codes, &SFI{OpCode: OpNotEq})
	case "&&":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicAnd})
	case "||":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicOr})
	case "!":
		y.codes = append(y.codes, &SFI{OpCode: OpLogicBang})
	default:
		panic(fmt.Sprintf("unknown operator: %s", i))
	}
}

func (y *SyntaxFlowVisitor) EmitAlert(ref string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpAlert,
		UnaryStr: ref,
	})
}

func (y *SyntaxFlowVisitor) EmitCheckParam(ref string, then string, elseString string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpCheckParams,
		UnaryStr: ref,
		Values:   []string{then, elseString},
	})
}

func (y *SyntaxFlowVisitor) EmitAddDescription(key string, value string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpAddDescription,
		UnaryStr: key,
		Values:   []string{key, value},
	})
}

func (v *SyntaxFlowVisitor) EmitPushGlob(i string) {
	v.codes = append(v.codes, &SFI{
		OpCode:   OpGlobMatch,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitRegexpMatch(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpReMatch,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitPushLiteral(i any) {
	switch ret := i.(type) {
	case string:
		v.codes = append(v.codes, &SFI{
			OpCode:   OpPushString,
			UnaryStr: ret,
		})
	case int:
		v.codes = append(v.codes, &SFI{
			OpCode:   OpPushNumber,
			UnaryInt: ret,
		})
	case bool:
		if ret {
			v.codes = append(v.codes, &SFI{
				OpCode:   OpPushBool,
				UnaryInt: 1,
			})
		} else {
			v.codes = append(v.codes, &SFI{
				OpCode:   OpPushString,
				UnaryInt: 0,
			})
		}
	default:
		panic(fmt.Sprintf("unknown type: %T", ret))
	}

}

func (v *SyntaxFlowVisitor) EmitCompareOpcode(i []string) {
	v.codes = append(v.codes, &SFI{
		OpCode: OpCompareOpcode,
		Values: i,
	})
}

const (
	CompareStringAnyMode  int = 0
	CompareStringHaveMode     = 1
)

func (v *SyntaxFlowVisitor) EmitCompareString(i []string, mode int) {
	v.codes = append(v.codes, &SFI{
		OpCode: OpCompareString,
		Values: lo.Map(i, func(item string, index int) string {
			if strings.HasPrefix(item, "'") && strings.HasSuffix(item, "'") {
				result, err := yakunquote.Unquote(item)
				if err != nil {
					return item
				}
				return result
			} else if strings.HasPrefix(item, `"`) && strings.HasSuffix(item, `"`) {
				result, err := yakunquote.Unquote(item)
				if err != nil {
					return item
				}
				return result
			} else {
				return item
			}
		}),
		UnaryInt: mode,
	})
}

func (v *SyntaxFlowVisitor) EmitCondition() {
	v.codes = append(v.codes, &SFI{
		OpCode: OpCondition,
	})
}

func (v *SyntaxFlowVisitor) EmitEqual(i any) {
	switch i.(type) {
	case string:
	case int:
	}
}
func (y *SyntaxFlowVisitor) EmitVersionIn( results ...*RecursiveConfigItem) {
	y.codes = append(y.codes, &SFI{
		OpCode: OpVersionIn,
		SyntaxFlowConfig: results,
	})
}

func (v *SyntaxFlowVisitor) EmitSearchExact(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpPushSearchExact,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitRecursiveSearchExact(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpRecursiveSearchExact,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitSearchGlob(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpPushSearchGlob,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitRecursiveSearchGlob(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpRecursiveSearchGlob,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitSearchRegexp(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpPushSearchRegexp,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitRecursiveSearchRegexp(mod int, i string) *SFI {
	sfi := &SFI{
		OpCode:   OpRecursiveSearchRegexp,
		UnaryStr: i,
		UnaryInt: mod,
	}
	v.codes = append(v.codes, sfi)
	return sfi
}

func (v *SyntaxFlowVisitor) EmitGetUsers() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetUsers})
}

func (v *SyntaxFlowVisitor) EmitGetDefs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetDefs})
}

func (v *SyntaxFlowVisitor) EmitGetBottomUsers(config ...*RecursiveConfigItem) {
	v.codes = append(v.codes, &SFI{
		OpCode:           OpGetBottomUsers,
		SyntaxFlowConfig: config,
	})
}

func (y *SyntaxFlowVisitor) EmitMergeRef(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpMergeRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitNativeCall(i string, results ...*RecursiveConfigItem) {
	y.codes = append(y.codes, &SFI{
		OpCode:           OpNativeCall,
		UnaryStr:         i,
		SyntaxFlowConfig: results,
	})
}

func (y *SyntaxFlowVisitor) EmitRemoveRef(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpRemoveRef,
		UnaryStr: i,
	})
}

func (y *SyntaxFlowVisitor) EmitIntersectionRef(i string) {
	y.codes = append(y.codes, &SFI{
		OpCode:   OpIntersectionRef,
		UnaryStr: i,
	})
}

func (v *SyntaxFlowVisitor) EmitGetTopDefs(config ...*RecursiveConfigItem) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetTopDefs, SyntaxFlowConfig: config})
}

func (v *SyntaxFlowVisitor) EmitPushCallArgs(i int) {
	v.codes = append(v.codes, &SFI{OpCode: OpGetCallArgs, UnaryInt: i})
}

func (v *SyntaxFlowVisitor) EmitDuplicate() {
	v.codes = append(v.codes, &SFI{OpCode: OpDuplicate})
}

func (v *SyntaxFlowVisitor) EmitGetCall() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetCall})
}

func (v *SyntaxFlowVisitor) EmitPushAllCallArgs() {
	v.codes = append(v.codes, &SFI{OpCode: OpGetAllCallArgs})
}

func (v *SyntaxFlowVisitor) Show() {
	for _, c := range v.codes {
		fmt.Println(c.String())
	}
}

func (v *SyntaxFlowVisitor) CreateFrame(vars *omap.OrderedMap[string, ValueOperator]) *SFFrame {
	return NewSFFrame(vars, v.rule.Content, v.codes)
}

func (y *SyntaxFlowVisitor) EmitPop() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpPop,
	})
}

func (y *SyntaxFlowVisitor) EmitListIndex(i int) {
	y.codes = append(y.codes, &SFI{OpCode: OpListIndex, UnaryInt: i})
}

func (v *SyntaxFlowVisitor) EmitPass() {
	v.codes = append(v.codes, &SFI{
		OpCode: OpPass,
	})
}

func (v *SyntaxFlowVisitor) EmitCreateIterator() *IterContext {
	idx := len(v.codes)
	it := &IterContext{start: idx}
	v.codes = append(v.codes, &SFI{OpCode: OpCreateIter, iter: it})
	return it
}

func (v *SyntaxFlowVisitor) EmitNextIterator(i *IterContext) {
	i.next = len(v.codes)
	v.codes = append(v.codes, &SFI{OpCode: OpIterNext, iter: i})
}

func (v *SyntaxFlowVisitor) EmitIterEnd(i *IterContext) {
	idx := len(v.codes)
	code := &SFI{OpCode: OpIterEnd, iter: i}
	i.end = idx
	v.codes = append(v.codes, code)
}

func (y *SyntaxFlowVisitor) EmitFilterExprEnter() *SFI {
	code := &SFI{OpCode: OpFilterExprEnter}
	y.codes = append(y.codes, code)
	return code
}

func (y *SyntaxFlowVisitor) EmitFilterExprExit(c *SFI) {
	idx := len(y.codes)
	c.UnaryInt = idx
	y.codes = append(y.codes, &SFI{
		OpCode:   OpFilterExprExit,
		UnaryInt: idx,
	})
}
func (y *SyntaxFlowVisitor) EmitCheckStackTop() {
	y.codes = append(y.codes, &SFI{
		OpCode: OpCheckStackTop,
	})
}

func (y *SyntaxFlowVisitor) EmitFileFilterReg(i string, m map[string]string, s []string) {
	y.codes = append(y.codes, &SFI{
		OpCode:               OpFileFilterReg,
		UnaryStr:             i,
		FileFilterMethodItem: m,
		Values:               s,
	})
}

func (y *SyntaxFlowVisitor) EmitFileFilterXpath(i string, m map[string]string, s []string) {
	y.codes = append(y.codes, &SFI{
		OpCode:               OpFileFilterXpath,
		UnaryStr:             i,
		FileFilterMethodItem: m,
		Values:               s,
	})
}

func (y *SyntaxFlowVisitor) EmitFileFilterJsonPath(i string, m map[string]string, s []string) {
	y.codes = append(y.codes, &SFI{
		OpCode:               OpFileFilterJsonPath,
		UnaryStr:             i,
		FileFilterMethodItem: m,
		Values:               s,
	})
}
