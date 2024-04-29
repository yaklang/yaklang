package go2ssa

import (
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"math"
	"strconv"
	"strings"
)

type ValueMap map[ssa.Value][]ssa.Value

func (y *builder) VisitLiteral(raw goparser.ILiteralContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if raw == nil || y == nil {
		return nil
	}

	i := raw.(*goparser.LiteralContext)
	if i == nil {
		return nil
	}

	if ret := i.BasicLit(); ret != nil {
		return y.VisitBasicLit(ret)
	} else if ret := i.CompositeLit(); ret != nil {
		return y.VisitCompositeLit(ret)
	} else if ret := i.FunctionLit(); ret != nil {
		return y.VisitFunctionLit(ret)
	}
	return nil
}

func (y *builder) VisitBasicLit(raw goparser.IBasicLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.BasicLitContext)
	if i == nil {
		return nil
	}

	if ret := i.NIL_LIT(); ret != nil {
		return y.ir.EmitConstInstNil()
	} else if ret := i.Integer(); ret != nil {
		return y.VisitInteger(ret)
	} else if ret := i.String_(); ret != nil {
		return y.VisitStringEx(ret)
	} else if ret := i.FLOAT_LIT(); ret != nil {
		lit := ret.GetText()
		f, _ := strconv.ParseFloat(lit, 64)
		return y.ir.EmitConstInst(f)
	} else if ret := i.IMAGINARY_LIT(); ret != nil {
		lit := ret.GetText()
		return y.ir.EmitConstInst(lit)
	} else if ret := i.RUNE_LIT(); ret != nil {
		lit := ret.GetText()
		if len(lit) != 3 {
			y.ir.NewError(ssa.Error, "go", "unsupport rune literal")
		}
		lit = lit[1:]
		runeLit := []rune(lit)
		return y.ir.EmitConstInst(runeLit[0])
	}

	y.ir.NewError(ssa.Error, "go", "cannot parse basic literal %v", i.GetText())
	return nil
}

func (y *builder) VisitCompositeLit(raw goparser.ICompositeLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.CompositeLitContext)
	if i == nil {
		return nil
	}

	litType := i.LiteralType().(*goparser.LiteralTypeContext)
	if litType == nil {
		return nil
	}

	var obj ssa.Value
	if ret := litType.StructType(); ret != nil {
		//todo 结构体
	} else if ret := litType.ArrayType(); ret != nil {
		arrayLength := ret.(*goparser.ArrayTypeContext).ArrayLength().GetText()
		typ := y.VisitElementType(ret.(*goparser.ArrayTypeContext).ElementType())

		if arrayLength != "" {
			length, err := strconv.Atoi(arrayLength)
			if err != nil {
				y.ir.NewError(ssa.Error, "go", "cannot parse array length %v", err)
			}
			// 初始化数组
			iniObj := y.ir.EmitMakeBuildWithType(
				ssa.NewSliceType(ssa.BasicTypes[ssa.NumberTypeKind]),
				y.ir.EmitConstInst(length),
				y.ir.EmitConstInst(length),
			)
			obj = y.VisitLiteralValue(i.LiteralValue(), iniObj)
			if typ.GetTypeKind() != ssa.SliceTypeKind {
				obj.SetType(typ)
			} else {
				coverType(obj.GetType(), typ)
			}
		} else {
			obj = y.VisitLiteralValue(i.LiteralValue(), nil)
			if typ.GetTypeKind() != ssa.SliceTypeKind {
				obj.SetType(typ)
			} else {
				coverType(obj.GetType(), typ)
			}
		}

	} else if ret := litType.ElementType(); ret != nil {
		obj = y.VisitLiteralValue(i.LiteralValue(), nil)
		typ := y.VisitElementType(ret)
		if obj.GetType().GetTypeKind() != ssa.SliceTypeKind {
			obj.SetType(typ)
		} else {
			coverType(obj.GetType(), typ)
		}
	} else if ret := litType.SliceType(); ret != nil {
		obj = y.VisitLiteralValue(i.LiteralValue(), nil)
		typ := y.VisitElementType(ret.(*goparser.SliceTypeContext).ElementType())
		if obj.GetType().GetTypeKind() != ssa.SliceTypeKind {
			obj.SetType(typ)
		} else {
			coverType(obj.GetType(), typ)
		}
	} else if ret := litType.MapType(); ret != nil {
		obj = y.VisitLiteralValue(i.LiteralValue(), nil) //todo 初始化map类型
		typ := y.VisitElementType(ret.(*goparser.MapTypeContext).ElementType())
		if obj.GetType().GetTypeKind() != ssa.SliceTypeKind {
			obj.SetType(typ)
		} else {
			coverType(obj.GetType(), typ)
		}
	} else if ret := litType.TypeName(); ret != nil {
		// todo 结构体字面量
	}

	return nil
}

func (y *builder) VisitFunctionLit(raw goparser.IFunctionLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.FunctionLitContext)
	if i == nil {
		return nil
	}

	return nil

}

func (y *builder) VisitInteger(raw goparser.IIntegerContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.IntegerContext)
	if i == nil {
		return nil
	}
	originIntStr := i.GetText()
	var resultInt64 int64
	var err error
	if ret := i.DECIMAL_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr, 10, 64)
	} else if ret := i.BINARY_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr[2:], 2, 64)
	} else if ret := i.OCTAL_LIT(); ret != nil {
		if strings.HasPrefix(ret.GetText(), "0o") || strings.HasPrefix(ret.GetText(), "0O") {
			resultInt64, err = strconv.ParseInt(originIntStr[2:], 8, 64)
		}
		resultInt64, err = strconv.ParseInt(originIntStr[1:], 8, 64)
	} else if ret := i.HEX_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr[2:], 16, 64)
	} else if ret := i.IMAGINARY_LIT(); ret != nil {
		return y.ir.EmitConstInst(ret.GetText())
	} else if ret := i.RUNE_LIT(); ret != nil {
		if len(originIntStr) != 3 {
			y.ir.NewError(ssa.Error, "go", "unsupport rune literal")
		}
		originIntStr = originIntStr[1:]
		runeLit := []rune(originIntStr)
		resultInt64 = int64(runeLit[0])
	}

	if err != nil {
		y.ir.NewError(ssa.Error, "go", "const parse %s as integer literal... is to large for int64: %v", originIntStr, resultInt64, err)
		return nil
	}
	if resultInt64 > math.MaxInt {
		return y.ir.EmitConstInst(int64(resultInt64))
	} else {
		return y.ir.EmitConstInst(int(resultInt64))
	}

}

func (y *builder) VisitStringEx(raw goparser.IString_Context) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.String_Context)
	if i == nil {
		return nil
	}

	text := i.GetText()
	if text == "" {
		return y.ir.EmitConstInst(text)
	}

	prefix := 0

	switch text[0] {
	case '"':
		var val string
		if lit := text; len(lit) >= 2 {
			val = lit[1 : len(lit)-1]
		} else {
			val = lit
		}
		return y.ir.EmitConstInstWithUnary(val, prefix)
	case '`':
		val := text[1 : len(text)-1]
		return y.ir.EmitConstInstWithUnary(val, prefix)
	default:
		y.ir.NewError(ssa.Error, "go", "unsupported string literal: %s", text)
		return nil
	}
}

func (y *builder) VisitLiteralValue(raw goparser.ILiteralValueContext, initObj *ssa.Make) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.LiteralValueContext)
	if i == nil {
		return nil
	}

	if i.ElementList() != nil {
		return y.VisitElementList(i.ElementList(), initObj)
	}
	return nil
}

func (y *builder) VisitElementList(raw goparser.IElementListContext, initObj *ssa.Make) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ElementListContext)
	if i == nil {
		return nil
	}

	creatObject := func(obj *ssa.Make, length int) ssa.Value {
		index := 0 //用以确认被赋值的索引位置
		for _, element := range i.AllKeyedElement() {
			if element != nil {
				key, expr := y.VisitKeyedElement(element)
				if index > length {
					y.ir.NewError(ssa.Error, "go", "index %v out of range %v", index, length)
					return nil
				}
				if key != nil {
					v := y.ir.CreateMemberCallVariable(obj, key)
					y.ir.AssignVariable(v, expr)
					index, err := strconv.Atoi(key.GetName())
					if err != nil {
						y.ir.NewError(ssa.Error, "go", "cannot parse key %v as integer", key.String())
					}
					index++ // 当以{2："a"，"b"}形式声明字面量，那么"b"的索引为3
				} else {
					// 以正常方式声明字面量{1,2,3}
					v := y.ir.CreateMemberCallVariable(obj, y.ir.EmitConstInst(index))
					y.ir.AssignVariable(v, expr)
					index++
				}
			}
		}
		return obj
	}

	if initObj == nil {
		length := len(i.AllKeyedElement())
		obj := y.ir.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.BasicTypes[ssa.AnyTypeKind]),
			y.ir.EmitConstInst(length), y.ir.EmitConstInst(length),
		)
		newObj := creatObject(obj, length)
		newObj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
		return newObj
	}

	length, err := strconv.Atoi(initObj.Len.GetName())
	if err != nil {
		y.ir.NewError(ssa.Error, "go", "cannot parse length %v as integer", initObj.Len.GetName())
	}

	newObj := creatObject(initObj, length)
	initObj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
	return newObj
}

func (y *builder) VisitKeyedElement(raw goparser.IKeyedElementContext) (key ssa.Value, value ssa.Value) {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil, nil
	}
	i := raw.(*goparser.KeyedElementContext)
	if i == nil {
		return nil, nil
	}

	if key := i.Key(); key != nil {
		if element := i.Element(); element != nil {
			keyExpr := y.VisitKey(key)
			eleExpr := y.VisitElement(element)
			return keyExpr, eleExpr
		}
	}
	eleExpr := y.VisitElement(i.Element())
	return nil, eleExpr
}

func (y *builder) VisitKey(raw goparser.IKeyContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.KeyContext)
	if i == nil {
		return nil
	}

	if i.IDENTIFIER() != nil {
		return y.ir.EmitConstInst(i.IDENTIFIER())
	} else if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	} else {
		y.ir.NewError(ssa.Error, "go", "unsupported key type: %s", i.GetText())
		return nil
	}
}

func (y *builder) VisitElement(raw goparser.IElementContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ElementContext)
	if i == nil {
		return nil
	}
	if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	} else if i.LiteralValue() != nil {
	}
	return nil
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
