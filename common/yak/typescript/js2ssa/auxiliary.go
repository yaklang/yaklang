package js2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
)

var arithmeticBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	// 普通算术操作
	ast.KindPlusToken:             ssa.OpAdd,
	ast.KindMinusToken:            ssa.OpSub,
	ast.KindAsteriskToken:         ssa.OpMul,
	ast.KindSlashToken:            ssa.OpDiv,
	ast.KindPercentToken:          ssa.OpMod,
	ast.KindAsteriskAsteriskToken: ssa.OpPow,

	// 算术赋值操作
	ast.KindPlusEqualsToken:             ssa.OpAdd,
	ast.KindMinusEqualsToken:            ssa.OpSub,
	ast.KindAsteriskEqualsToken:         ssa.OpMul,
	ast.KindSlashEqualsToken:            ssa.OpDiv,
	ast.KindPercentEqualsToken:          ssa.OpMod,
	ast.KindAsteriskAsteriskEqualsToken: ssa.OpPow,
}

var bitwiseBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	// 普通按位操作
	ast.KindAmpersandToken:                         ssa.OpAnd,
	ast.KindBarToken:                               ssa.OpOr,
	ast.KindCaretToken:                             ssa.OpXor,
	ast.KindLessThanLessThanToken:                  ssa.OpShl,
	ast.KindGreaterThanGreaterThanToken:            ssa.OpShr,
	ast.KindGreaterThanGreaterThanGreaterThanToken: ssa.OpShr,

	// 按位赋值操作
	ast.KindAmpersandEqualsToken:                         ssa.OpAnd,
	ast.KindBarEqualsToken:                               ssa.OpOr,
	ast.KindCaretEqualsToken:                             ssa.OpXor,
	ast.KindLessThanLessThanEqualsToken:                  ssa.OpShl,
	ast.KindGreaterThanGreaterThanEqualsToken:            ssa.OpShr,
	ast.KindGreaterThanGreaterThanGreaterThanEqualsToken: ssa.OpShr,
}
var comparisonBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	ast.KindLessThanToken:                ssa.OpLt,
	ast.KindGreaterThanToken:             ssa.OpGt,
	ast.KindLessThanEqualsToken:          ssa.OpLtEq,
	ast.KindGreaterThanEqualsToken:       ssa.OpGtEq,
	ast.KindEqualsEqualsToken:            ssa.OpEq,
	ast.KindEqualsEqualsEqualsToken:      ssa.OpEq,
	ast.KindExclamationEqualsToken:       ssa.OpNotEq,
	ast.KindExclamationEqualsEqualsToken: ssa.OpNotEq,
}

var logicalBinOpTbl = map[ast.Kind]ssa.BinaryOpcode{
	// 普通逻辑操作
	ast.KindAmpersandAmpersandToken: ssa.OpLogicAnd,
	ast.KindBarBarToken:             ssa.OpLogicOr,

	// 逻辑赋值操作
	ast.KindAmpersandAmpersandEqualsToken: ssa.OpLogicAnd,
	ast.KindBarBarEqualsToken:             ssa.OpLogicOr,
}

// VisitLeftValueExpression 只接收左值
func (b *builder) VisitLeftValueExpression(node *ast.Expression) *ssa.Variable {
	lval, _ := b.VisitExpression(node, true)
	return lval
}

// VisitRightValueExpression 只接收右值
func (b *builder) VisitRightValueExpression(node *ast.Expression) ssa.Value {
	_, rval := b.VisitExpression(node, false)
	return rval
}

func (b *builder) IsNullishValue(value ssa.Value) bool {
	return value == nil || value.IsUndefined() || value == ssa.NewNil()
}
