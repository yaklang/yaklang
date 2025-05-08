package js2ssa

import (
	"fmt"
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

func (b *builder) GetLabelByName(name string) *ssa.LabelBuilder {
	label, ok := b.labels[name]
	if !ok || label == nil {
		return nil
	}
	return label
}

func (b *builder) handlerGoto(labelName string, isBreak ...bool) {
	gotoBuilder := b.BuildGoto(labelName)
	if len(isBreak) > 0 {
		gotoBuilder.SetBreak(isBreak[0])
	}
	if targetBlock := b.GetLabel(labelName); targetBlock != nil {
		// target label exist, just set it
		LabelBuilder := b.GetLabelByName(labelName)
		if LabelBuilder == nil {
			b.NewError(ssa.Error, TAG, fmt.Sprintf("label: %s not found", labelName))
			return
		}
		gotoBuilder.SetLabel(targetBlock)
		f := gotoBuilder.Finish()
		LabelBuilder.SetGotoFinish(f)
	} else {
		// target label not exist, create it
		LabelBuilder := b.BuildLabel(labelName)
		// use handler function
		LabelBuilder.SetGotoHandler(func(_goto *ssa.BasicBlock) {
			gotoBuilder.SetLabel(_goto)
			f := gotoBuilder.Finish()
			LabelBuilder.SetGotoFinish(f)
		})
		b.labels[labelName] = LabelBuilder
	}
}
