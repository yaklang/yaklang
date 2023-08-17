package ssa

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// slice literal
func (b *builder) buildSliceLiteral(stmt *yak.SliceLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	_s := stmt.ExpressionListMultiline()
	if _s == nil {
		b.NewError(Warn, "slice literal not have expression")
		return b.emitInterfaceBuildWithType(nil, NewConst(0), NewConst(0))
	}
	s, ok := _s.(*yak.ExpressionListMultilineContext)
	if !ok {
		b.NewError(Error, "slice literal expression parse error")
		return nil
	}
	vs := b.buildExpressionListMultiline(s)
	return b.CreateInterfaceWithVs(nil, vs)
}

// slice typed literal
func (b *builder) buildSliceTypedLiteral(stmt *yak.SliceTypedLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	return nil
}

// map literal
func (b *builder) buildMapLiteral(stmt *yak.MapLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	if s := stmt.MapTypedLiteral(); s != nil {
		if s, ok := s.(*yak.MapTypedLiteralContext); ok {
			b.buildMapTypedLiteral(s)
		} else {
			b.NewError(Error, "map typed literal parse error")
		}
	}

	_s := stmt.MapPairs()
	if _s == nil {
		b.NewError(Warn, "map literal not have map pairs")
		return b.emitInterfaceBuildWithType(nil, NewConst(0), NewConst(0))
	}
	s, ok := _s.(*yak.MapPairsContext)
	if !ok {
		b.NewError(Error, "map literal map pairs parse error")
		return nil
	}
	allPair := s.AllMapPair()

	// itf :=
	keys := make([]Value, 0, len(allPair))
	values := make([]Value, 0, len(allPair))
	for _, p := range allPair {
		p := p.(*yak.MapPairContext)
		keys = append(keys, b.buildExpression(p.Expression(0).(*yak.ExpressionContext)))
		values = append(values, b.buildExpression(p.Expression(1).(*yak.ExpressionContext)))
	}
	return b.CreateInterfaceWithVs(keys, values)
}

// map typed literal
func (b *builder) buildMapTypedLiteral(stmt *yak.MapTypedLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	return nil
}
