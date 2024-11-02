// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package spelparser // SpelParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by SpelParser.
type SpelParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SpelParser#script.
	VisitScript(ctx *ScriptContext) interface{}

	// Visit a parse tree produced by SpelParser#spelExpr.
	VisitSpelExpr(ctx *SpelExprContext) interface{}

	// Visit a parse tree produced by SpelParser#node.
	VisitNode(ctx *NodeContext) interface{}

	// Visit a parse tree produced by SpelParser#nonDottedNode.
	VisitNonDottedNode(ctx *NonDottedNodeContext) interface{}

	// Visit a parse tree produced by SpelParser#dottedNode.
	VisitDottedNode(ctx *DottedNodeContext) interface{}

	// Visit a parse tree produced by SpelParser#functionOrVar.
	VisitFunctionOrVar(ctx *FunctionOrVarContext) interface{}

	// Visit a parse tree produced by SpelParser#methodArgs.
	VisitMethodArgs(ctx *MethodArgsContext) interface{}

	// Visit a parse tree produced by SpelParser#args.
	VisitArgs(ctx *ArgsContext) interface{}

	// Visit a parse tree produced by SpelParser#methodOrProperty.
	VisitMethodOrProperty(ctx *MethodOrPropertyContext) interface{}

	// Visit a parse tree produced by SpelParser#projection.
	VisitProjection(ctx *ProjectionContext) interface{}

	// Visit a parse tree produced by SpelParser#selection.
	VisitSelection(ctx *SelectionContext) interface{}

	// Visit a parse tree produced by SpelParser#startNode.
	VisitStartNode(ctx *StartNodeContext) interface{}

	// Visit a parse tree produced by SpelParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by SpelParser#numericLiteral.
	VisitNumericLiteral(ctx *NumericLiteralContext) interface{}

	// Visit a parse tree produced by SpelParser#parenspelExpr.
	VisitParenspelExpr(ctx *ParenspelExprContext) interface{}

	// Visit a parse tree produced by SpelParser#typeReference.
	VisitTypeReference(ctx *TypeReferenceContext) interface{}

	// Visit a parse tree produced by SpelParser#possiblyQualifiedId.
	VisitPossiblyQualifiedId(ctx *PossiblyQualifiedIdContext) interface{}

	// Visit a parse tree produced by SpelParser#nullReference.
	VisitNullReference(ctx *NullReferenceContext) interface{}

	// Visit a parse tree produced by SpelParser#constructorReference.
	VisitConstructorReference(ctx *ConstructorReferenceContext) interface{}

	// Visit a parse tree produced by SpelParser#constructorArgs.
	VisitConstructorArgs(ctx *ConstructorArgsContext) interface{}

	// Visit a parse tree produced by SpelParser#inlineListOrMap.
	VisitInlineListOrMap(ctx *InlineListOrMapContext) interface{}

	// Visit a parse tree produced by SpelParser#listBindings.
	VisitListBindings(ctx *ListBindingsContext) interface{}

	// Visit a parse tree produced by SpelParser#listBinding.
	VisitListBinding(ctx *ListBindingContext) interface{}

	// Visit a parse tree produced by SpelParser#mapBindings.
	VisitMapBindings(ctx *MapBindingsContext) interface{}

	// Visit a parse tree produced by SpelParser#mapBinding.
	VisitMapBinding(ctx *MapBindingContext) interface{}

	// Visit a parse tree produced by SpelParser#beanReference.
	VisitBeanReference(ctx *BeanReferenceContext) interface{}

	// Visit a parse tree produced by SpelParser#inputParameter.
	VisitInputParameter(ctx *InputParameterContext) interface{}

	// Visit a parse tree produced by SpelParser#propertyPlaceHolder.
	VisitPropertyPlaceHolder(ctx *PropertyPlaceHolderContext) interface{}
}
