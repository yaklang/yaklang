// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package spelparser // SpelParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseSpelParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSpelParserVisitor) VisitScript(ctx *ScriptContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitSpelExpr(ctx *SpelExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitNode(ctx *NodeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitNonDottedNode(ctx *NonDottedNodeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitDottedNode(ctx *DottedNodeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitFunctionOrVar(ctx *FunctionOrVarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitMethodArgs(ctx *MethodArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitArgs(ctx *ArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitMethodOrProperty(ctx *MethodOrPropertyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitProjection(ctx *ProjectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitSelection(ctx *SelectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitStartNode(ctx *StartNodeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitNumericLiteral(ctx *NumericLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitParenspelExpr(ctx *ParenspelExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitTypeReference(ctx *TypeReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitPossiblyQualifiedId(ctx *PossiblyQualifiedIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitNullReference(ctx *NullReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitConstructorReference(ctx *ConstructorReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitConstructorArgs(ctx *ConstructorArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitInlineListOrMap(ctx *InlineListOrMapContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitListBindings(ctx *ListBindingsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitListBinding(ctx *ListBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitMapBindings(ctx *MapBindingsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitMapBinding(ctx *MapBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitBeanReference(ctx *BeanReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitInputParameter(ctx *InputParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSpelParserVisitor) VisitPropertyPlaceHolder(ctx *PropertyPlaceHolderContext) interface{} {
	return v.VisitChildren(ctx)
}
