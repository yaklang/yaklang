package go2ssa

import (
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitExpression(raw goparser.IExpressionContext) ssa.Value {
	if raw == nil || y == nil {
		return nil
	}
	if raw.GetText() == "" {
		return nil
	}
	switch ret := raw.(type) {
	case *goparser.PrimaryExpressionContext:
		return y.VisitPrimaryExpression(ret.PrimaryExpr())
	case *goparser.UnaryExpressionContext:
		return y.VisitUnaryExpression(ret.UnaryExpr())
	case *goparser.ArithmeticExpressionContext:
	case *goparser.ComparisonExpressionContext:
	case *goparser.BitwiseExpressionContext:
	}
	return nil
}

func (y *builder) VisitPrimaryExpression(raw goparser.IPrimaryExprContext) ssa.Value {
	if raw == nil || y == nil {
		return nil
	}
	primaryExpr := raw.(*goparser.PrimaryExprContext)
	if primaryExpr == nil {
		return nil
	}
	return nil
}

func (y *builder) VisitUnaryExpression(raw goparser.IUnaryExprContext) ssa.Value {
	if raw == nil || y == nil {
		return nil
	}
	i := raw.(*goparser.UnaryExprContext)
	if i == nil {
		return nil
	}
	value := y.VisitExpression(i.Expression())
	switch {
	case i.PLUS() != nil:
		return y.ir.EmitUnOp(ssa.OpPlus, value)
	case i.MINUS() != nil:
		return y.ir.EmitUnOp(ssa.OpNeg, value)
	case i.EXCLAMATION() != nil:
		return y.ir.EmitUnOp(ssa.OpNot, value)
	case i.CARET() != nil:
		return y.ir.EmitUnOp(ssa.OpBitwiseNot, value)
	case i.STAR() != nil:
	case i.AMPERSAND() != nil:
	case i.RECEIVE() != nil:
		return y.ir.EmitUnOp(ssa.OpChan, value)
	default:
		return nil
	}
	return nil
}

func (y *builder) VisitOperandName(raw goparser.IOperandNameContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.OperandNameContext)
	if i == nil {
		return nil
	}
	// syntax a.b
	return nil
}

func (y *builder) VisitQualifiedIdent() ssa.Value {
	return nil
}
func (y *builder) VisitOperand(raw goparser.IOperandContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.OperandContext)
	if i == nil {
		return nil
	}
	switch {
	case i.Literal() != nil:
		y.VisitLiteral(i.Literal())
	case i.OperandName() != nil:
		return y.VisitOperandName(i.OperandName())
	case i.Expression() != nil:
		return y.VisitExpression(i.Expression())
	}
	return nil
}

func (y *builder) VisitConversion(raw goparser.IConversionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ConversionContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitType_(raw goparser.IType_Context) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.Type_Context)
	if i == nil {
		return nil
	}
	switch {
	case i.TypeName() != nil:
		return y.VisitTypename(i.TypeName())
	case i.TypeLit() != nil:

	case i.Type_() != nil:
		return y.VisitType_(i.Type_())
	}
	return nil
}

func (y *builder) VisitTypename(raw goparser.ITypeNameContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.TypeNameContext)
	if i == nil {
		return nil
	}
	switch {
	case i.IDENTIFIER() != nil:
		return ssa.GetType(i.IDENTIFIER().GetText())
	case i.QualifiedIdent() != nil:
	}
	return nil
}
func (y *builder) VisitTypeLit(raw goparser.ITypeLitContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.TypeLitContext)
	if i == nil {
		return nil
	}
	switch {
	case i.ArrayType() != nil:
		return y.VisitArrayType(i.ArrayType())
	case i.StructType() != nil:
		return ssa.NewStructType()
	case i.PointerType() != nil:
		//todo
	case i.FunctionType() != nil:
	case i.InterfaceType() != nil:
	case i.SliceType() != nil:
	case i.MapType() != nil:
	case i.ChannelType() != nil:
	}
	return nil
}

func (y *builder) VisitArrayType(raw goparser.IArrayTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ArrayTypeContext)
	if i == nil {
		return nil
	}
	_ = i.ArrayLength().(*goparser.ArrayLengthContext)
	elementType := y.VisitElementType(i.ElementType())
	return ssa.NewSliceType(elementType)
}

func (y *builder) VisitFunctionType(raw goparser.IFunctionTypeContext) (params []ssa.Type, results []ssa.Type) {
	if y == nil || raw == nil {
		return nil, nil
	}
	i := raw.(*goparser.FunctionTypeContext)
	if i == nil {
		return nil, nil
	}
	return nil, nil
}
func (y *builder) VisitResult(raw goparser.IResultContext) []ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ResultContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitParameters(raw goparser.IParametersContext) (params []string, types []ssa.Type) {
	if y == nil || raw == nil {
		return nil, nil
	}
	i := raw.(*goparser.ParametersContext)
	if i == nil {
		return nil, nil
	}

	return nil, nil
}
func (y *builder) VisitParameterDecl(raw goparser.IParameterDeclContext) ([]string, bool, []ssa.Type) {
	if y == nil || raw == nil {
		return nil, false, nil
	}
	i := raw.(*goparser.ParameterDeclContext)
	if i == nil {
		return nil, false, nil
	}
	return nil, false, nil
}
func (y *builder) VisitElementType(raw goparser.IElementTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ElementTypeContext)
	if i == nil {
		return nil
	}
	if types := ssa.GetTypeByStr(raw.GetText()); types != nil {
		return types
	}
	//如果不是基础类型，会有问题
	return nil
}
