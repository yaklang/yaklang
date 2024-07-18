package go2ssa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildOperandExpression(exp gol.IOperandContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	
	if !IslValue { // right
		if literal := exp.(*gol.OperandContext).Literal(); literal != nil {
			return b.buildLiteral(literal),nil
		}
		if id := exp.(*gol.OperandContext).OperandName(); id != nil {
			return b.buildOperandNameR(id),nil
		}
		if e := exp.(*gol.OperandContext).Expression(); e != nil {
			return b.buildExpression(e.(*gol.ExpressionContext), false)
		}
	}else{ // left
		if id := exp.(*gol.OperandContext).OperandName(); id != nil {
			return nil, b.buildOperandNameL(id)
		}
	}
	return nil, nil
}

func (b* astbuilder) buildOperandNameL(name gol.IOperandNameContext) (*ssa.Variable) {
    if id := name.(*gol.OperandNameContext).IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}
		v := b.CreateVariable(text)
		return v
    }
	return nil
}

func (b* astbuilder) buildOperandNameR(name gol.IOperandNameContext) (ssa.Value) {
    if id := name.(*gol.OperandNameContext).IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}
		v := b.ReadValue(text)
		return v

    }
	return nil
}

func (b *astbuilder) buildLiteral(exp gol.ILiteralContext) (ssa.Value) {

	if lit := exp.(*gol.LiteralContext).BasicLit(); lit != nil {
		return b.buildBasicLit(lit.(*gol.BasicLitContext))
	}

	if lit := exp.(*gol.LiteralContext).CompositeLit(); lit != nil {
	    return b.buildCompositeLit(lit.(*gol.CompositeLitContext))
	}
	return nil
}

func (b *astbuilder) buildCompositeLit(exp *gol.CompositeLitContext) (ssa.Value) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var values []ssa.Value

	typ, lenv := b.buildTypeLiteral(exp.LiteralType().(*gol.LiteralTypeContext))
	if value := exp.LiteralValue(); value != nil {
		if s, ok := value.(*gol.LiteralValueContext); ok {
			values = b.buildLiteralValue(s, typ)
		}
	}

	if lenv != nil {
	    maxlen , _ := strconv.ParseInt(lenv.String(), 10, 64)
		if int(maxlen) < len(values) {
		    b.NewError(ssa.Error, TAG, "index %d is out of bounds (>= %d)", int(maxlen),len(values))
			return nil
		}
	}

	zero := b.EmitConstInst(0)
	switch typ.GetTypeKind() {
	case ssa.SliceTypeKind, ssa.BytesTypeKind:
		if len(values) == 0 {
			return b.EmitMakeBuildWithType(typ, zero, zero)
		} else {
			return b.CreateInterfaceWithSlice(values)
		}

	}

	return nil
}

func (b *astbuilder) buildLiteralValue(exp *gol.LiteralValueContext, typ ssa.Type) ([]ssa.Value) {
    recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var values []ssa.Value

	if list := exp.ElementList(); list != nil {
	    for _, e := range list.(*gol.ElementListContext).AllKeyedElement() {
			tmps := b.buildKeyedElement(e.(*gol.KeyedElementContext), typ)
			values = append(values, tmps...)
		}
	}

	return values
}

func (b *astbuilder) buildKeyedElement(exp *gol.KeyedElementContext, typ ssa.Type) ([]ssa.Value) {
    recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if elem := exp.Element(); elem != nil {
		if s, ok := elem.(*gol.ElementContext); ok {
			return b.buildElement(s, typ)
		}
	}

	return nil
}

func (b *astbuilder) buildElement(exp *gol.ElementContext, typ ssa.Type) ([]ssa.Value) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if e := exp.Expression(); e != nil {
	    right,_ := b.buildExpression(e.(*gol.ExpressionContext) , false)
		return []ssa.Value{right}  
	}

	if e := exp.LiteralValue(); e != nil {
	    return b.buildLiteralValue(e.(*gol.LiteralValueContext), typ)
	}

	return nil
}

func (b *astbuilder) buildTypeLiteral(stmt *gol.LiteralTypeContext) (ssa.Type,ssa.Value) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	text := stmt.GetText()
	// var type name
	if b := ssa.GetTypeByStr(text); b != nil {
		return b,nil
	}

	// slice type literal
	if s, ok := stmt.SliceType().(*gol.SliceTypeContext); ok {
		return b.buildSliceTypeLiteral(s),nil
	}

	// array type literal
	if s, ok := stmt.ArrayType().(*gol.ArrayTypeContext); ok {
	    return b.buildArrayTypeLiteral(s)
	}

	// map type literal
	if strings.HasPrefix(text, "map") {
		if s, ok := stmt.MapType().(*gol.MapTypeContext); ok {
			return b.buildMapTypeLiteral(s),nil
		}
	}

	// struct type literal
	if strings.HasPrefix(text, "struct") {
		if s, ok := stmt.StructType().(*gol.StructTypeContext); ok {
			_ = s
		}
	}

	return nil,nil
}

func (b *astbuilder) buildMapTypeLiteral (stmt *gol.MapTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var keyTyp ssa.Type
	var valueTyp ssa.Type
	if s, ok := stmt.Type_().(*gol.Type_Context); ok {
		keyTyp = b.buildType(s)
	}

	// value
	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		valueTyp = b.buildType(s.Type_().(*gol.Type_Context))
	}
	if keyTyp != nil && valueTyp != nil {
		return ssa.NewMapType(keyTyp, valueTyp)
	}

    return nil
}


func (b *astbuilder) buildSliceTypeLiteral(stmt *gol.SliceTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if stmt.GetText() == "[]byte" || stmt.GetText() == "[]uint8" {
		return ssa.BasicTypes[ssa.BytesTypeKind]
	}
	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		if eleTyp := b.buildType(s.Type_().(*gol.Type_Context)); eleTyp != nil {
			return ssa.NewSliceType(eleTyp)
		}
	}
	return nil
}


func (b *astbuilder) buildArrayTypeLiteral(stmt *gol.ArrayTypeContext) (ssa.Type,ssa.Value) {
    recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var value ssa.Value

	if s, ok := stmt.ArrayLength().(*gol.ArrayLengthContext); ok {
	    if e := s.Expression(); e != nil {
	        rightv , _ := b.buildExpression(e.(*gol.ExpressionContext), false)
			value = rightv
	    }
	}

	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		if eleTyp := b.buildType(s.Type_().(*gol.Type_Context)); eleTyp != nil {
			return ssa.NewSliceType(eleTyp),value
		}
	}
	return nil,nil
}


func (b *astbuilder) buildType(typ *gol.Type_Context) ssa.Type {
    recoverRange := b.SetRange(typ.BaseParserRuleContext)
	defer recoverRange()
	var typeType ssa.Type 
	if name := typ.TypeName();name != nil {
	    typeType = ssa.GetTypeByStr(typ.GetText())
	}
	return typeType
}

func (b *astbuilder) buildBasicLit(exp *gol.BasicLitContext) (ssa.Value) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if lit := exp.Integer(); lit != nil {
		return b.buildIntegerLiteral(lit.(*gol.IntegerContext))
	}

	if lit := exp.NIL_LIT(); lit != nil {
		return b.EmitConstInstNil()
	}

	if lit := exp.FLOAT_LIT(); lit != nil {
		return b.EmitConstInst(lit.GetText())
	}

	if lit := exp.String_(); lit != nil {
		return b.buildStringLiteral(lit.(*gol.String_Context))
	}

	if lit := exp.Char_(); lit != nil {
	    return b.buildCharLiteral(lit.(*gol.Char_Context))
	}

	return nil
}

func (b *astbuilder) buildStringLiteral(stmt *gol.String_Context) ssa.Value {
	var text = stmt.GetText()
	if text == "" {
		return b.EmitConstInst(text)
	}

	switch text[0] {
	case '"':
		val, err := strconv.Unquote(text)
		if err != nil {
			b.NewError(ssa.Error, TAG, "cannot parse string literal: %s failed: %s", stmt.GetText(), err.Error())
		}
		return b.EmitConstInstWithUnary(val, 0)
	case '`':
		// TODO
	}

	return nil
}

func (b *astbuilder) buildCharLiteral(stmt *gol.Char_Context) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	lit := stmt.GetText()
	var s string
	var err error
	if lit == "'\\''" {
		s = "'"
	} else {
		lit = strings.ReplaceAll(lit, `"`, `\"`)
		s, err = strconv.Unquote(fmt.Sprintf("\"%s\"", lit[1:len(lit)-1]))
		if err != nil {
			b.NewError(ssa.Error, TAG, "unquote error %s", err)
			return nil
		}
	}
	runeChar := []rune(s)[0]
	if runeChar < 256 {
		return b.EmitConstInst(byte(runeChar))
	} else {
		return b.EmitConstInst(runeChar)
	}
}

func (b *astbuilder) buildIntegerLiteral(stmt *gol.IntegerContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	lit := stmt.GetText()

	if find := strings.Contains(lit, "."); find {
		var f, _ = strconv.ParseFloat(lit, 64)
		return b.EmitConstInst(f)
	} else {
		var err error
		var originStr = stmt.GetText()
		var intStr = strings.ToLower(originStr)
		var resultInt64 int64

		if num := stmt.DECIMAL_LIT(); num != nil { // 十进制
			if strings.Contains(stmt.GetText(), "e") {
				var f, _ = strconv.ParseFloat(intStr, 64)
				return b.EmitConstInst(f)
			}
			resultInt64, err = strconv.ParseInt(intStr, 10, 64)
		} else if num := stmt.HEX_LIT(); num != nil { // 十六进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 16, 64)
		} else if num := stmt.BINARY_LIT(); num != nil { // 二进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 2, 64)
		} else if num := stmt.OCTAL_LIT(); num != nil { // 八进制
			resultInt64, err = strconv.ParseInt(intStr[2:], 8, 64)
		} else {
			b.NewError(ssa.Error, TAG, "cannot parse num for literal: %s", stmt.GetText())
			return nil
		}

		if err != nil {
			b.NewError(ssa.Error, TAG, "const parse %s as integer literal... is to large for int64: %v", originStr, err)
			return nil
		}

		if resultInt64 > math.MaxInt {
			return b.EmitConstInst(int64(resultInt64))
		} else {
			return b.EmitConstInst(int64(resultInt64))
		}
	}
}