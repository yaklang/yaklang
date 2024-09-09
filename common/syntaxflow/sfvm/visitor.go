package sfvm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"github.com/yaklang/yaklang/common/syntaxflow/sf"
)

type SyntaxFlowVisitor struct {
	text               string
	title              string
	allowIncluded      string
	rawDesc            map[string]string
	description        string
	purpose            string
	severity           string
	language           string
	verifyFilesystem   map[string]string
	negativeFilesystem map[string]string
	codes              []*SFI
}

func NewSyntaxFlowVisitor() *SyntaxFlowVisitor {
	sfv := &SyntaxFlowVisitor{
		verifyFilesystem:   make(map[string]string),
		negativeFilesystem: make(map[string]string),
		rawDesc:            make(map[string]string),
	}
	return sfv
}

func (y *SyntaxFlowVisitor) VisitFlow(raw sf.IFlowContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*sf.FlowContext)
	if i == nil {
		return
	}

	statements, _ := i.Statements().(*sf.StatementsContext)
	if statements == nil {
		return
	}

	for _, stmt := range statements.AllStatement() {
		y.VisitStatement(stmt)
	}
	return
}

func (y *SyntaxFlowVisitor) VisitStatement(raw sf.IStatementContext) {
	if y == nil || raw == nil {
		return
	}

	switch i := raw.(type) {
	case *sf.FilterContext:
		y.EmitEnterStatement()
		y.VisitFilterStatement(i.FilterStatement())
		y.EmitExitStatement()
	case *sf.CheckContext:
		y.VisitCheckStatement(i.CheckStatement())
	case *sf.DescriptionContext:
		y.VisitDescriptionStatement(i.DescriptionStatement())
	case *sf.AlertContext:
		y.VisitAlertStatement(i.AlertStatement())
	case *sf.EmptyContext:
		return
	case *sf.FileFilterContentContext:
		err := y.VisitFileFilterContent(i.FileFilterContentStatement())
		if err != nil {
			log.Warnf("visit case *sf.FileFilterContentContext: failed: %s", err)
		}
	default:
		log.Debugf("syntaxflow met statement: %v", strings.TrimSpace(i.GetText()))
	}
	return
}

func (y *SyntaxFlowVisitor) VisitFilterStatement(raw sf.IFilterStatementContext) {
	if y == nil || raw == nil {
		return
	}

	switch i := raw.(type) {
	case *sf.PureFilterExprContext:
		y.EmitCheckStackTop()
		expr := i.FilterExpr()
		if expr == nil {
			return
		}
		err := y.VisitFilterExpr(expr)
		if err != nil {
			msg := fmt.Sprintf("parse expr: %v failed: %s", i.FilterExpr().GetText(), err)
			log.Error(msg)
			panic(msg)
		}
		// collect result for variable or save to '_' variable
		if up := i.RefVariable(); up != nil {
			varName := y.VisitRefVariable(up) // create symbol and pop stack
			y.EmitUpdate(varName)
		} else {
			y.EmitPop()
		}
	case *sf.RefFilterExprContext:
		if ref := i.RefVariable(0); ref != nil {
			variable := y.VisitRefVariable(ref)
			y.EmitNewRef(variable)
		} else {
			panic("BUG: ref filter expr1 is nil")
		}

		enter := y.EmitFilterExprEnter()
		for _, filter := range i.AllFilterItem() {
			y.VisitFilterItem(filter)
		}
		y.EmitFilterExprExit(enter)

		// collect result for variable or save to '_' variable
		if up := i.RefVariable(1); up != nil {
			varName := y.VisitRefVariable(up) // create symbol and pop stack
			y.EmitUpdate(varName)
		} else {
			y.EmitPop()
		}
	}

	// for filter expression

}

func (y *SyntaxFlowVisitor) VisitRefVariable(raw sf.IRefVariableContext) string {
	if y == nil || raw == nil {
		return ""
	}
	i, _ := raw.(*sf.RefVariableContext)
	if i == nil {
		return ""
	}
	return i.Identifier().GetText()
}

func (y *SyntaxFlowVisitor) VisitChainFilter(raw sf.IChainFilterContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	//switch ret := raw.(type) {
	//case *sf.FlatContext:
	//	var count int
	//	l := len(ret.AllFilters())
	//	y.EmitFlatStart(l)
	//	for _, filter := range ret.AllFilters() {
	//		count++
	//		y.VisitFilters(filter)
	//		y.EmitRestoreFlatContext()
	//	}
	//	y.EmitFlatDone(count)
	//case *sf.BuildMapContext:
	//	var count int
	//	y.EmitMapBuildStart()
	//	l := len(ret.AllColon())
	//	var vals []string = make([]string, l)
	//	for i := 0; i < l; i++ {
	//		key := ret.Identifier(i).GetText()
	//		count++
	//		y.EmitNewRef(key)
	//		vals[i] = key
	//		y.VisitFilters(ret.Filters(i))
	//		y.EmitWithdraw()
	//		y.EmitUpdate(key)
	//		y.EmitRestoreMapContext()
	//		// pop val, create object and set key
	//	}
	//	y.EmitMapBuildDone(vals...)
	//default:
	//	panic("Unexpected VisitChainFilter")
	//}
	//
	//return nil
	return nil
}

func (y *SyntaxFlowVisitor) VisitConditionExpression(raw sf.IConditionExpressionContext) any {
	if y == nil || raw == nil {
		return nil
	}

	switch i := raw.(type) {
	case *sf.FilterConditionContext:
		ctx := y.EmitCreateIterator()
		y.EmitNextIterator(ctx)
		err := y.VisitFilterExpr(i.FilterExpr())
		if err != nil {
			log.Warnf("compile filter-expr in condition expression failed: %v", err)
			return err
		}
		y.EmitIterEnd(ctx)
	case *sf.OpcodeTypeConditionContext:
		y.EmitDuplicate()
		opcodes := i.AllOpcodesCondition()
		ops := make([]string, 0, len(opcodes))
		for _, opcode := range opcodes {
			text := yakunquote.TryUnquote(opcode.GetText())
			switch text {
			case "add":
				text = "+"
			case "sub":
				text = "-"
			}
			switch text {
			case "call":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeCall])
			case "phi":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodePhi])
			case "const", "constant":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeConstInst])
			case "param", "formal_param":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeParameter])
			case "return":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeReturn])
			case "function", "func", "def":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeFunction])
			case "+", "-", "*", "/", "%":
				ops = append(ops, ssa.SSAOpcode2Name[ssa.SSAOpcodeBinOp]+"["+text+"]")
			default:
				log.Errorf("unknown opcode: %s", opcode.GetText())
			}
		}
		y.EmitCompareOpcode(ops)
	case *sf.StringContainAnyConditionContext:
		y.EmitDuplicate()
		res := y.VisitStringLiteralWithoutStarGroup(i.StringLiteralWithoutStarGroup())
		y.EmitCompareString(res, CompareStringAnyMode)
	case *sf.StringContainHaveConditionContext:
		y.EmitDuplicate()
		res := y.VisitStringLiteralWithoutStarGroup(i.StringLiteralWithoutStarGroup())
		y.EmitCompareString(res, CompareStringHaveMode)
	case *sf.FilterExpressionCompareContext:
		if i.NumberLiteral() != nil {
			n := y.VisitNumberLiteral(i.NumberLiteral())
			y.EmitPushLiteral(n)
		} else if i.Identifier() != nil {
			y.EmitPushLiteral(i.Identifier().GetText())
		} else {
			if i.GetText() == "true" {
				y.EmitPushLiteral(true)
			} else {
				y.EmitPushLiteral(false)
			}
		}
		y.EmitOperator(i.GetOp().GetText())
	case *sf.FilterExpressionRegexpMatchContext:
		if i.StringLiteral() != nil {
			r, glob := y.VisitStringLiteral(i.StringLiteral())
			if glob {
				y.EmitPushGlob(r)
				if i.GetOp().GetTokenType() == sf.SyntaxFlowLexerNotRegexpMatch {
					y.EmitOperator("!")
				}
				return nil
			} else {
				r, err := regexp.Compile(regexp.QuoteMeta(r))
				if err != nil {
					panic("golang regexp: regexp compile failed: " + err.Error())
				}
				y.EmitRegexpMatch(r.String())
				return nil
			}
		}

		if i.RegexpLiteral() != nil {
			result := i.RegexpLiteral().GetText()
			result = result[1 : len(result)-1]
			result = strings.ReplaceAll(result, `\/`, `/`)
			re, err := regexp.Compile(result)
			if err != nil {
				panic("golang regexp: regexp compile failed: " + err.Error())
			}
			y.EmitRegexpMatch(result)
			if i.GetOp().GetTokenType() == sf.SyntaxFlowLexerNotRegexpMatch {
				y.EmitOperator("!")
			}
			return re
		}
		panic("BUG: in regexp match")
	case *sf.FilterExpressionAndContext:
		for _, exp := range i.AllConditionExpression() {
			y.VisitConditionExpression(exp)
		}
		y.EmitOperator("&&")
	case *sf.FilterExpressionOrContext:
		for _, exp := range i.AllConditionExpression() {
			y.VisitConditionExpression(exp)
		}
		y.EmitOperator("||")
	case *sf.NotConditionContext:
		y.VisitConditionExpression(i.ConditionExpression())
		y.EmitOperator("!")
	case *sf.ParenConditionContext:
		y.VisitConditionExpression(i.ConditionExpression())
	default:
		log.Errorf("unexpected condition expression: %T", i)
	}

	return nil
}

const tmpPH = "__[[PLACEHOLDER]]__"

func (y *SyntaxFlowVisitor) VisitStringLiteral(raw sf.IStringLiteralContext) (string, bool) {
	if y == nil || raw == nil {
		return "", false
	}

	i, _ := raw.(*sf.StringLiteralContext)
	if i == nil {
		return "", false
	}

	var text = i.GetText()
	return y.FormatStringOrGlob(text)
}

func (y *SyntaxFlowVisitor) FormatStringOrGlob(text string) (string, bool) {
	return text, strings.Contains(text, "*")
}

func (y *SyntaxFlowVisitor) VisitNumberLiteral(raw sf.INumberLiteralContext) int {
	if y == nil || raw == nil {
		return -1
	}

	i, _ := raw.(*sf.NumberLiteralContext)
	if i == nil {
		return -1
	}

	result := strings.ToLower(i.GetText())
	switch {
	case strings.HasPrefix(result, "0b"):
		result, err := strconv.ParseInt(result[2:], 2, 64)
		if err != nil {
			panic(err)
		}
		return int(result)
	case strings.HasSuffix(result, "0x"):
		result, err := strconv.ParseInt(result[:len(result)-1], 16, 64)
		if err != nil {
			panic(err)
		}
		return int(result)
	case strings.HasSuffix(result, "0o"):
		result, err := strconv.ParseInt(result[:len(result)-1], 8, 64)
		if err != nil {
			panic(err)
		}
		return int(result)
	default:
		if ret := strings.TrimLeft(result, "0"); ret != "" {
			result, err := strconv.ParseInt(ret, 10, 64)
			if err != nil {
				panic(err)
			}
			return int(result)
		} else {
			return 0
		}
	}

	return -1
}

func (y *SyntaxFlowVisitor) VisitStringLiteralWithoutStarGroup(raw sf.IStringLiteralWithoutStarGroupContext) []string {
	if y == nil || raw == nil {
		return []string{}
	}

	i, _ := raw.(*sf.StringLiteralWithoutStarGroupContext)
	if i == nil {
		return []string{}
	}

	res := make([]string, 0, len(i.AllStringLiteralWithoutStar()))
	for _, s := range i.AllStringLiteralWithoutStar() {
		res = append(res, s.GetText())
	}
	return res
}
