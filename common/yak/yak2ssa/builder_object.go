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
		// b.NewError(ssa.Warn, TAG, "slice literal not have expression")
		return b.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.CreateAnyType()),
			b.EmitConstInstPlaceholder(0), b.EmitConstInstPlaceholder(0),
		)
	}
	s, ok := _s.(*yak.ExpressionListMultilineContext)
	if !ok {
		// b.NewError(ssa.Error, TAG, "slice literal expression parse error")
		return nil
	}
	allExpr := s.AllExpression()

	obj := b.InterfaceAddFieldBuild(len(allExpr),
		func(i int) ssa.Value { return b.EmitConstInstPlaceholder(i) },
		func(i int) ssa.Value {
			return b.buildExpression(allExpr[i].(*yak.ExpressionContext))
		},
	)
	obj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
	return obj
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
		typ := b.buildSliceTypeLiteral(s)
		if typ.GetTypeKind() != ssa.SliceTypeKind {
			// []number may be ByteTypeKind
			slice.SetType(typ)
		} else {
			coverType(slice.GetType(), typ)
		}
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
			ssa.NewMapType(ssa.CreateAnyType(), ssa.CreateAnyType()),
			b.EmitConstInstPlaceholder(0), b.EmitConstInstPlaceholder(0),
		)
	}
	s, ok := _s.(*yak.MapPairsContext)
	if !ok {
		b.NewError(ssa.Error, TAG, "map literal map pairs parse error")
		return nil
	}
	allPair := s.AllMapPair()

	obj := b.InterfaceAddFieldBuild(len(allPair),
		func(i int) ssa.Value {
			return b.buildExpression(allPair[i].(*yak.MapPairContext).Expression(0).(*yak.ExpressionContext))
		},
		func(i int) ssa.Value {
			return b.buildExpression(allPair[i].(*yak.MapPairContext).Expression(1).(*yak.ExpressionContext))
		},
	)
	t := obj.GetType().(*ssa.ObjectType)
	var fieldTyp ssa.Type = ssa.CreateAnyType()
	var keyTyp ssa.Type = ssa.CreateAnyType()
	if t.FieldType != nil {
		fieldTyp = t.FieldType
	}
	if t.KeyTyp != nil {
		keyTyp = t.KeyTyp
	}

	coverType(obj.GetType(), ssa.NewMapType(keyTyp, fieldTyp))

	return obj
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
		typ := b.buildMapTypeLiteral(s)
		coverType(maps.GetType(), typ)
	} else {
		b.NewError(ssa.Warn, TAG, "map type not set")
	}

	return maps
}

func coverType(ityp, iwantTyp ssa.Type) {
	typ, ok := ityp.(*ssa.ObjectType)
	if !ok {
		return
	}
	wantTyp, ok := iwantTyp.(*ssa.ObjectType)
	if !ok {
		return
	}

	typ.SetTypeKind(wantTyp.GetTypeKind())
	switch wantTyp.GetTypeKind() {
	case ssa.SliceTypeKind:
		typ.FieldType = wantTyp.FieldType
	case ssa.MapTypeKind:
		typ.FieldType = wantTyp.FieldType
		typ.KeyTyp = wantTyp.KeyTyp
	}
}
