package yak2ssa

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ExpressionListMultiline interface {
	ExpressionListMultiline() yak.IExpressionListMultilineContext
}

func (b *astbuilder) buildSliceFromExprList(stmt ExpressionListMultiline) ssa.Value {
	_s := stmt.ExpressionListMultiline()
	if _s == nil {
		b.NewError(ssa.Warn, TAG, "slice literal not have expression")
		return b.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.BasicTypes[ssa.Any]),
			ssa.NewConst(0), ssa.NewConst(0),
		)
	}
	s, ok := _s.(*yak.ExpressionListMultilineContext)
	if !ok {
		b.NewError(ssa.Error, TAG, "slice literal expression parse error")
		return nil
	}
	vs := b.buildExpressionListMultiline(s)
	return b.CreateInterfaceWithVs(nil, vs)
}

// slice literal
func (b *astbuilder) buildSliceLiteral(stmt *yak.SliceLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	return b.buildSliceFromExprList(stmt)
}

// slice typed literal
func (b *astbuilder) buildSliceTypedLiteral(stmt *yak.SliceTypedLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	slice := b.buildSliceFromExprList(stmt)

	if s, ok := stmt.SliceTypeLiteral().(*yak.SliceTypeLiteralContext); ok {
		slice.SetType(b.buildSliceTypeLiteral(s))
	} else {
		b.NewError(ssa.Warn, TAG, "slice type not set")
	}

	return slice
}

type MapPairs interface {
	MapPairs() yak.IMapPairsContext
}

func (b *astbuilder) buildMapFromMapPairs(stmt MapPairs) ssa.Value {
	_s := stmt.MapPairs()
	if _s == nil {
		b.NewError(ssa.Warn, TAG, "map literal not have map pairs")
		return b.EmitMakeBuildWithType(
			ssa.NewMapType(ssa.BasicTypes[ssa.Any], ssa.BasicTypes[ssa.Any]),
			ssa.NewConst(0), ssa.NewConst(0),
		)
	}
	s, ok := _s.(*yak.MapPairsContext)
	if !ok {
		b.NewError(ssa.Error, TAG, "map literal map pairs parse error")
		return nil
	}
	allPair := s.AllMapPair()

	// itf :=
	keys := make([]ssa.Value, 0, len(allPair))
	values := make([]ssa.Value, 0, len(allPair))
	for _, p := range allPair {
		p := p.(*yak.MapPairContext)
		keys = append(keys, b.buildExpression(p.Expression(0).(*yak.ExpressionContext)))
		values = append(values, b.buildExpression(p.Expression(1).(*yak.ExpressionContext)))
	}
	return b.CreateInterfaceWithVs(keys, values)
}

// map literal
func (b *astbuilder) buildMapLiteral(stmt *yak.MapLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s := stmt.MapTypedLiteral(); s != nil {
		if s, ok := s.(*yak.MapTypedLiteralContext); ok {
			return b.buildMapTypedLiteral(s)
		} else {
			b.NewError(ssa.Error, TAG, "map typed literal parse error")
		}
	}
	return b.buildMapFromMapPairs(stmt)
}

// map typed literal
func (b *astbuilder) buildMapTypedLiteral(stmt *yak.MapTypedLiteralContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	maps := b.buildMapFromMapPairs(stmt)

	if s, ok := stmt.MapTypeLiteral().(*yak.MapTypeLiteralContext); ok {
		maps.SetType(b.buildMapTypeLiteral(s))
	} else {
		b.NewError(ssa.Warn, TAG, "map type not set")
	}

	return maps
}
