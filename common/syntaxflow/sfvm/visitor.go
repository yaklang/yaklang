package sfvm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
)

type SyntaxFlowVisitor struct {
	rule         *schema.SyntaxFlowRule
	verifyFsInfo []*VerifyFsInfo
	codes        []*SFI
}

type VerifyFsInfo struct {
	language           string
	rawDesc            map[string]string
	verifyFilesystem   map[string]string
	negativeFilesystem map[string]string
}

func NewExtraDesc() *VerifyFsInfo {
	return &VerifyFsInfo{
		rawDesc:            make(map[string]string),
		verifyFilesystem:   make(map[string]string),
		negativeFilesystem: make(map[string]string),
	}
}

func NewSyntaxFlowVisitor() *SyntaxFlowVisitor {
	sfv := &SyntaxFlowVisitor{
		rule: &schema.SyntaxFlowRule{
			AlertDesc: make(schema.MapEx[string, *schema.SyntaxFlowDescInfo]),
		},
		verifyFsInfo: make([]*VerifyFsInfo, 0),
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

	statement := y.EmitEnterStatement()
	switch i := raw.(type) {
	case *sf.FilterContext:
		y.VisitFilterStatement(i.FilterStatement())
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
	y.EmitExitStatement(statement)
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

		enter := y.EmitEnterStatement()
		for _, filter := range i.AllFilterItem() {
			y.VisitFilterItem(filter)
		}
		y.EmitExitStatement(enter)

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
		y.EmitOpEmptyCompare()
		ctx := y.EmitCreateIterator()
		y.EmitNextIterator(ctx)
		err := y.VisitFilterExpr(i.FilterExpr())
		if err != nil {
			log.Warnf("compile filter-expr in condition expression failed: %v", err)
			return err
		}
		y.EmitDuplicate()
		y.EmitOpCheckEmpty(ctx)
		y.EmitLatchIterator(ctx)
		y.EmitIterEnd(ctx)
	case *sf.OpcodeTypeConditionContext:
		opcodes := i.AllOpcodesCondition()
		ops := make([]string, 0, len(opcodes))
		for _, opcode := range opcodes {
			ops = append(ops, opcode.GetText())
		}
		y.EmitCompareOpcode(ops)
	case *sf.StringContainAnyConditionContext:
		res := y.VisitStringLiteralWithoutStarGroup(i.StringLiteralWithoutStarGroup())
		y.EmitCompareString(res, MatchHaveAny)
	case *sf.StringContainHaveConditionContext:
		res := y.VisitStringLiteralWithoutStarGroup(i.StringLiteralWithoutStarGroup())
		y.EmitCompareString(res, MatchHave)
	case *sf.FilterExpressionCompareContext:
		if i.NumberLiteral() != nil {
			n := y.VisitNumberLiteral(i.NumberLiteral())
			y.EmitPushLiteral(n)
		} else if i.Identifier() != nil {
			y.EmitPushLiteral(yakunquote.TryUnquote(i.Identifier().GetText()))
		} else {
			if yakunquote.TryUnquote(i.GetText()) == "true" {
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
	case *sf.VersionInConditionContext:
		y.VisitVersionInExpression(i.VersionInExpression())
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

func (y *SyntaxFlowVisitor) VisitStringLiteralWithoutStarGroup(raw sf.IStringLiteralWithoutStarGroupContext) []func() (string, ConditionFilterMode) {
	var result []func() (string, ConditionFilterMode)
	if y == nil || raw == nil {
		return result
	}

	i, _ := raw.(*sf.StringLiteralWithoutStarGroupContext)
	if i == nil {
		return result
	}

	for _, s := range i.AllStringLiteralWithoutStar() {
		star := s.(*sf.StringLiteralWithoutStarContext)
		result = append(result, func() (string, ConditionFilterMode) {
			var (
				mode = ExactConditionFilter
				text = star.GetText()
			)

			if star.RegexpLiteral() != nil {
				mode = RegexpConditionFilter
				text = strings.TrimSuffix(strings.TrimPrefix(star.RegexpLiteral().GetText(), "/"), "/")
			} else if glob, b := y.FormatStringOrGlob(star.GetText()); b {
				text = glob
				mode = GlobalConditionFilter
			}
			return text, mode
		})
	}
	return result
}

func (y *SyntaxFlowVisitor) VisitVersionInExpression(raw sf.IVersionInExpressionContext) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*sf.VersionInExpressionContext)
	if i == nil {
		return
	}
	for i, interval := range i.AllVersionInterval() {
		y.VisitVersionInterval(interval)
		if i != 0 {
			y.EmitOperator("||")
		}
	}
}

func (y *SyntaxFlowVisitor) VisitVersionInterval(raw sf.IVersionIntervalContext) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*sf.VersionIntervalContext)
	if i == nil {
		return
	}
	var left, right, vstart, vend string
	if i.ListSelectOpen() != nil {
		left = "greaterEqual"
	} else if i.OpenParen() != nil {
		left = "greaterThan"
	}

	if i.ListSelectClose() != nil {
		right = "lessEqual"
	} else if i.CloseParen() != nil {
		right = "lessThan"
	}

	if v := i.Vstart(); v != nil {
		vstart = y.VisitVersionString(v.(*sf.VstartContext).VersionString())
	}
	if v := i.Vend(); v != nil {
		vend = y.VisitVersionString(v.(*sf.VendContext).VersionString())
	}

	y.EmitVersionIn(&RecursiveConfigItem{
		Key:   left,
		Value: vstart,
	}, &RecursiveConfigItem{
		Key:   right,
		Value: vend,
	})
}

func (y *SyntaxFlowVisitor) VisitVersionString(raw sf.IVersionStringContext) string {
	if y == nil || raw == nil {
		return ""
	}
	i := raw.(*sf.VersionStringContext)
	if i == nil {
		return ""
	}
	return i.GetText()
}
