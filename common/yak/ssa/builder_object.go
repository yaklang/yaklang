package ssa

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (b *builder) buildSliceLiteral(ctx *yak.SliceLiteralContext) Value {
	slm := ctx.ExpressionListMultiline()
	if slm == nil {
		return NewConst(make([]any, 0))
	}

	var ssaVals []Value
	for _, exprIface := range slm.(*yak.ExpressionListMultilineContext).AllExpression() {
		if exprIface == nil {
			continue
		}
		expr := exprIface.(*yak.ExpressionContext)
		ssaVals = append(ssaVals, b.buildExpression(expr))
	}
	panic("UNIMPLEMENTED build SliceLiteral")
	return NewConst(nil)
}

func (b *builder) buildMapLiteral(ctx *yak.MapLiteralContext) Value {
	rawPairs := ctx.MapPairs()
	if rawPairs == nil {
		return NewConst(make(map[string]any))
	}
	pairs, ok := ctx.MapPairs().(*yak.MapPairsContext)
	if !ok {
		log.Errorf("buildMapLiteral error! parse MapPairs failed: %v", ctx.GetText())
		return NewConst(make(map[string]any))
	}

	var keyTypes Types
	var valueTypes Types
	_ = keyTypes
	_ = valueTypes
	for _, pair := range pairs.AllMapPair() {
		p := pair.(*yak.MapPairContext)
		if p == nil {
			continue
		}
		keySSAVal := b.buildExpression(p.Expression(0).(*yak.ExpressionContext))
		valueSSAVal := b.buildExpression(p.Expression(1).(*yak.ExpressionContext))
		spew.Dump(keySSAVal, valueSSAVal)
	}

	panic("UNIMPLEMENTED build MapLiteral")
	return NewConst(nil)
}
