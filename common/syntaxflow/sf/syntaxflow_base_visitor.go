// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseSyntaxFlowVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSyntaxFlowVisitor) VisitFlow(ctx *FlowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilters(ctx *FiltersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterStatement(ctx *FilterStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitExistedRef(ctx *ExistedRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRefVariable(ctx *RefVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDirectionFilter(ctx *DirectionFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldFilter(ctx *FieldFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitAheadChainFilter(ctx *AheadChainFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitParenFilter(ctx *ParenFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepChainFilter(ctx *DeepChainFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitPrimaryFilter(ctx *PrimaryFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNumberIndexFilter(ctx *NumberIndexFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldChainFilter(ctx *FieldChainFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFlat(ctx *FlatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitBuildMap(ctx *BuildMapContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterFieldMember(ctx *FilterFieldMemberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpression(ctx *FilterExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNumberLiteral(ctx *NumberLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTypeCast(ctx *TypeCastContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTypes(ctx *TypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitBoolLiteral(ctx *BoolLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
