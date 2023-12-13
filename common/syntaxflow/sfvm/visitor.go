package sfvm

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"strings"
)

type SyntaxFlowVisitor struct {
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

	if i.ExistedRef() != nil {
		y.VisitExistedRef(i.ExistedRef())
	}

	if ret := i.GetDirection(); ret != nil {
		if ret.GetText() == ">>" {
			// >>
			// y.EmitDirection("right")
		} else {
			// <<
			// y.EmitDirection("left")
		}
	} else {
		// <<
		// y.EmitDirection("left")
	}

	y.VisitFilterExpr(i.FilterExpr())

	if i.Filter() != nil {
		varName := y.VisitRefVariable(i.RefVariable()) // create symbol and pop stack
		_ = varName
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitExistedRef(raw sf.IExistedRefContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.ExistedRefContext)
	if i == nil {
		return nil
	}

	var varName = y.VisitRefVariable(i.RefVariable())
	_ = varName
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

func (y *SyntaxFlowVisitor) VisitFilterExpr(raw sf.IFilterExprContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.FilterExprContext)
	if i == nil {
		return nil
	}

	switch ret := raw.(type) {
	case *sf.PrimaryFilterContext:
		ret.Identifier().GetText() // emit field
	case *sf.NumberIndexFilterContext:
		y.VisitNumberLiteral(ret.NumberLiteral()) // emit index number
	case *sf.DirectionFilterContext:
		if ret.GetOp().GetText() == "<<" {

		} else {

		}
		y.VisitFilterExpr(ret.FilterExpr())
	case *sf.ParenFilterContext:
		y.VisitFilterExpr(ret.FilterExpr())
	case *sf.FieldFilterContext:
		y.VisitFilterFieldMember(ret.FilterFieldMember()) // emit field or cast type
	case *sf.AheadChainFilterContext:
		y.VisitFilterExpr(ret.FilterExpr())
		y.VisitChainFilter(ret.ChainFilter())
		// ahead push

	case *sf.DeepChainFilterContext:
		y.VisitFilterExpr(ret.FilterExpr())
		y.VisitChainFilter(ret.ChainFilter())
		// head

	case *sf.FieldChainFilterContext:
		y.VisitFilterExpr(ret.FilterExpr())
		y.VisitFilterFieldMember(ret.FilterFieldMember()) // emit field or cast type
	default:
		panic("BUG: in filterExpr")
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitChainFilter(raw sf.IChainFilterContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	switch ret := raw.(type) {
	case *sf.FlatContext:
		for _, filter := range ret.AllFilterExpression() {
			y.VisitFilterExpression(filter)
		}
	case *sf.BuildMapContext:
		for i := 0; i < len(ret.AllColon()); i++ {
			key := ret.Identifier(i).GetText()
			val := y.VisitFilters(ret.Filters(i))
			_, _ = key, val
			// pop val, create object and set key
		}
	default:
		panic("Unexpected VisitChainFilter")
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterExpression(raw sf.IFilterExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterFieldMember(raw sf.IFilterFieldMemberContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*sf.FilterFieldMemberContext)
	if i == nil {
		return nil
	}

	return nil
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
	case strings.HasSuffix(result, "0x"):
	case strings.HasSuffix(result, "0b"):
	}

	return -1
}
