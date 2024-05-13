package sfvm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sf"
)

type SyntaxFlowVisitor struct {
	text  string
	codes []*SFI
}

func NewSyntaxFlowVisitor() *SyntaxFlowVisitor {
	sfv := &SyntaxFlowVisitor{}
	return sfv
}

func (y *SyntaxFlowVisitor) VisitFlow(raw sf.IFlowContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.FlowContext)
	if i == nil {
		return nil
	}

	return y.VisitFilters(i.Filters())
}

func (y *SyntaxFlowVisitor) VisitFilters(raw sf.IFiltersContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.FiltersContext)
	if i == nil {
		return nil
	}

	for _, stmt := range i.AllFilterStatement() {
		y.EmitPushInput()
		y.VisitFilterStatement(stmt)
	}
	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterStatement(raw sf.IFilterStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.FilterStatementContext)
	if i == nil {
		return nil
	}

	err := y.VisitFilterExpr(i.FilterExpr())
	if err != nil {
		msg := fmt.Sprintf("parse expr: %v failed: %s", i.FilterExpr().GetText(), err)
		panic(msg)
	}

	if i.RefVariable() != nil {
		varName := y.VisitRefVariable(i.RefVariable()) // create symbol and pop stack
		y.EmitUpdate(varName)
	}

	return nil
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

func (y *SyntaxFlowVisitor) VisitConditionExpression(raw sf.IConditionExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	switch i := raw.(type) {
	case *sf.FilterExpressionNumberContext:
		y.EmitPushLiteral(y.VisitNumberLiteral(i.NumberLiteral()))
		y.EmitOperator("==")
	case *sf.FilterExpressionStringContext:
		text, globMode := y.VisitStringLiteral(i.StringLiteral())
		if !globMode {
			y.EmitPushLiteral(text)
			y.EmitOperator("==")
		} else {
			y.EmitPushGlob(text)
		}
	case *sf.FilterExpressionRegexpContext:
		result := i.RegexpLiteral().GetText()
		result = result[1 : len(result)-1]
		result = strings.ReplaceAll(result, `\/`, `/`)
		re, err := regexp.Compile(result)
		if err != nil {
			panic("golang regexp: regexp compile failed: " + err.Error())
		}
		y.EmitRegexpMatch(result)
		return re
	case *sf.FilterExpressionParenContext:
		return y.VisitConditionExpression(i.ConditionExpression())
	case *sf.FilterExpressionNotContext:
		y.VisitConditionExpression(i.ConditionExpression())
		y.EmitOperator("!")
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
