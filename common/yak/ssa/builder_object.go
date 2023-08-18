package ssa

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

type ExpressionListMultiline interface {
	ExpressionListMultiline() yak.IExpressionListMultilineContext
}

func (b *builder) buildSliceFromExprList(stmt ExpressionListMultiline) Value {
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

// slice literal
func (b *builder) buildSliceLiteral(stmt *yak.SliceLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()
	return b.buildSliceFromExprList(stmt)
}

// slice typed literal
func (b *builder) buildSliceTypedLiteral(stmt *yak.SliceTypedLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	slice := b.buildSliceFromExprList(stmt)

	if s, ok := stmt.SliceTypeLiteral().(*yak.SliceTypeLiteralContext); ok {
		typ := b.buildSliceTypeLiteral(s)
		slice.SetType(Types{typ})
	} else {
		b.NewError(Warn, "slice type not set")
	}

	return slice
}

type MapPairs interface {
	MapPairs() yak.IMapPairsContext
}

func (b *builder) buildMapFromMapPairs(stmt MapPairs) Value {
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

// map literal
func (b *builder) buildMapLiteral(stmt *yak.MapLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	if s := stmt.MapTypedLiteral(); s != nil {
		if s, ok := s.(*yak.MapTypedLiteralContext); ok {
			return b.buildMapTypedLiteral(s)
		} else {
			b.NewError(Error, "map typed literal parse error")
		}
	}
	return b.builder.buildMapFromMapPairs(stmt)
}

// map typed literal
func (b *builder) buildMapTypedLiteral(stmt *yak.MapTypedLiteralContext) Value {
	recover := b.SetRange(stmt.BaseParserRuleContext)
	defer recover()

	maps := b.buildMapFromMapPairs(stmt)

	if s, ok := stmt.MapTypeLiteral().(*yak.MapTypeLiteralContext); ok {
		typ := b.buildMapTypeLiteral(s)
		maps.SetType(Types{typ})
	} else {
		b.NewError(Warn, "map type not set")

	}

	return maps
}
