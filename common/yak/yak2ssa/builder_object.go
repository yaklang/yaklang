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
			ssa.NewSliceType(ssa.BasicTypes[ssa.AnyTypeKind]),
			b.EmitConstInst(0), b.EmitConstInst(0),
		)
	}
	s, ok := _s.(*yak.ExpressionListMultilineContext)
	if !ok {
		// b.NewError(ssa.Error, TAG, "slice literal expression parse error")
		return nil
	}
	vs := b.buildExpressionListMultiline(s)
	obj := b.CreateInterfaceWithVs(nil, vs)
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
			ssa.NewMapType(ssa.BasicTypes[ssa.AnyTypeKind], ssa.BasicTypes[ssa.AnyTypeKind]),
			b.EmitConstInst(0), b.EmitConstInst(0),
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
	obj := b.CreateInterfaceWithVs(keys, values)
	t := obj.GetType().(*ssa.ObjectType)
	var fieldTyp ssa.Type = ssa.BasicTypes[ssa.AnyTypeKind]
	var keyTyp ssa.Type = ssa.BasicTypes[ssa.AnyTypeKind]
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
