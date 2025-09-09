package go2ssa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildBoolLiteral(name string) ssa.Value {
	boolLit, err := strconv.ParseBool(name)
	if err != nil {
		b.NewError(ssa.Error, TAG, UnhandledBool())
	}
	return b.EmitConstInst(boolLit)
}

func (b *astbuilder) buildLiteral(exp *gol.LiteralContext) ssa.Value {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if lit := exp.BasicLit(); lit != nil {
		return b.buildBasicLit(lit.(*gol.BasicLitContext))
	}

	if lit := exp.CompositeLit(); lit != nil {
		return b.buildCompositeLit(lit.(*gol.CompositeLitContext))
	}

	if lit := exp.FunctionLit(); lit != nil {
		return b.buildFunctionLit(lit.(*gol.FunctionLitContext))
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
}

func (b *astbuilder) buildFunctionLit(exp *gol.FunctionLitContext) ssa.Value {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	newFunc := b.NewFunc("")

	hitDefinedFunction := false
	MarkedFunctionType := b.GetMarkedFunction()
	handleFunctionType := func(fun *ssa.Function) {
		fun.ParamLength = len(fun.Params)
		if MarkedFunctionType == nil {
			return
		}
		if len(fun.Params) != len(MarkedFunctionType.Parameter) {
			return
		}

		for i, p := range fun.Params {
			val, ok := fun.GetValueById(p)
			if !ok {
				continue
			}
			val.SetType(MarkedFunctionType.Parameter[i])
		}
		hitDefinedFunction = true
	}

	{
		recoverRange := b.SetRange(exp.BaseParserRuleContext)
		b.FunctionBuilder = b.PushFunction(newFunc)
		b.SupportClosure = true
		b.SetForceCapture(true)

		if para, ok := exp.Signature().(*gol.SignatureContext); ok {
			b.buildSignature(para)
		}

		handleFunctionType(b.Function)

		b.SetGlobal = false
		if block, ok := exp.Block().(*gol.BlockContext); ok {
			b.buildBlock(block, true)
		}

		b.Finish()
		b.SetForceCapture(false)
		b.SupportClosure = false
		b.FunctionBuilder = b.PopFunction()
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}
		recoverRange()
	}

	return newFunc
}

type keyValue struct {
	key   ssa.Value
	value ssa.Value
	kv    []keyValue
}

func (b *astbuilder) buildCompositeLit(exp *gol.CompositeLitContext) ssa.Value {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	var kvs []keyValue

	typ := b.buildLiteralType(exp.LiteralType().(*gol.LiteralTypeContext))
	if value := exp.LiteralValue(); value != nil {
		if s, ok := value.(*gol.LiteralValueContext); ok {
			switch t := typ.(type) {
			case *ssa.ObjectType:
				if t.GetTypeKind() == ssa.StructTypeKind {
					kvs = b.buildLiteralValue(s, true)
				} else if t.GetTypeKind() == ssa.SliceTypeKind {
					kvs = b.buildLiteralValue(s, false)
				} else {
					kvs = b.buildLiteralValue(s, false)
				}
			case *ssa.AliasType: // 处理golang库
				typ = typ.(*ssa.AliasType).GetType()
				kvs = b.buildLiteralValue(s, true)
			default:
				typ = ssa.CreateAnyType()
				kvs = b.buildLiteralValue(s, true)
			}
		}
	}

	var typeHandler func(ssa.Type, []keyValue) ssa.Value

	typeHandler = func(typ ssa.Type, kvs []keyValue) ssa.Value {
		var obj ssa.Value

		switch typ.GetTypeKind() {
		case ssa.SliceTypeKind, ssa.BytesTypeKind:
			if len(kvs) == 0 {
				obj = b.CreateObjectWithMap(nil, nil)
				obj.SetType(typ)
				return obj
			}
			if kvs[0].value != nil {
				return kvs[0].value
			}

			objt := typ.(*ssa.ObjectType)
			obj = b.InterfaceAddFieldBuild(len(kvs),
				func(i int) ssa.Value {
					return b.EmitConstInst(i)
				},
				func(i int) ssa.Value {
					return typeHandler(objt.FieldType, kvs[i].kv)
				})
		case ssa.MapTypeKind:
			if len(kvs) == 0 {
				obj = b.CreateObjectWithMap(nil, nil)
				obj.SetType(typ)
				return obj
			}
			if kvs[0].value != nil {
				return kvs[0].value
			}
			objt := typ.(*ssa.ObjectType)
			obj = b.InterfaceAddFieldBuild(len(kvs),
				func(i int) ssa.Value {
					return kvs[i].key
				},
				func(i int) ssa.Value {
					return typeHandler(objt.FieldType, kvs[i].kv)
				})
		case ssa.StructTypeKind:
			objt := typ.(*ssa.ObjectType)

			fullInit := func() {
				obj = b.InterfaceAddFieldBuild(len(objt.Keys),
					func(i int) ssa.Value {
						if i < len(objt.Keys) {
							return objt.Keys[i]
						} else {
							return b.EmitConstInst("")
						}
					},
					func(i int) ssa.Value {
						return typeHandler(objt.FieldTypes[i], kvs[i].kv)
					})
			}

			partInit := func() {
				obj = b.InterfaceAddFieldBuild(len(objt.Keys),
					func(i int) ssa.Value {
						return objt.Keys[i]
					},
					func(i int) ssa.Value {
						for y, kv := range kvs {
							if objt.Keys[i].String() == kv.key.String() {
								return typeHandler(objt.FieldTypes[i], kvs[y].kv)
							}
						}
						return b.GetDefaultValue(objt.FieldTypes[i])
					})
			}

			if len(kvs) == 0 {
				partInit()
				return obj
			}

			if kvs[0].value != nil {
				// todo: 只有指针才会复用object，目前默认非指针
				if m, ok := kvs[0].value.(*ssa.Make); ok {
					var mkeys, mmembers []ssa.Value
					for k, m := range m.GetAllMember() {
						mkeys = append(mkeys, k)
						mmembers = append(mmembers, m)
					}
					newObject := b.InterfaceAddFieldBuild(len(mkeys),
						func(i int) ssa.Value {
							return mkeys[i]
						},
						func(i int) ssa.Value {
							return mmembers[i]
						})
					return newObject
				}

				return kvs[0].value
			}

			if kvs[0].key == nil { // 全部初始化
				fullInit()
			} else { // 部分初始化
				partInit()
				for _, kv := range kvs {
					if a, ok := objt.AnonymousField[kv.key.String()]; ok {
						newObject := typeHandler(a, kv.kv)
						variable := b.CreateMemberCallVariable(obj, b.EmitConstInst(kv.key.String()))
						b.AssignVariable(variable, newObject)
					}
				}
			}
		case ssa.InterfaceTypeKind:
			// TODO
			obj = b.InterfaceAddFieldBuild(0,
				func(i int) ssa.Value {
					return b.EmitConstInst(i)
				},
				func(i int) ssa.Value {
					return b.EmitConstInst(i)
				})
		case ssa.AliasTypeKind:
			alias := typ.(*ssa.AliasType)
			obj = typeHandler(alias.GetType(), kvs)
		case ssa.AnyTypeKind: // 对于未知类型，这里选择根据LiteralValue的特征来推测其类型
			var typt ssa.Type

			if len(kvs) == 0 {
				return b.EmitUndefined(typ.String())
			}
			if kvs[0].value != nil {
				return kvs[0].value
			}

			if kvs[0].key == nil && kvs[0].value == nil { // array slice
				kv := kvs[0].kv
				typt = ssa.NewSliceType(kv[0].value.GetType())
			} else if kvs[0].key == nil { // any
				return b.EmitUndefined(typ.String())
			} else if _, ok := ssa.ToBasicType(kvs[0].key.GetType()); ok { // struct map
				typt = ssa.NewStructType()
				for _, kv := range kvs {
					value := kv.kv[0].value
					typt.(*ssa.ObjectType).AddField(kv.key, value.GetType())
				}
			} else {
				return b.EmitUndefined(typt.String())
			}

			return typeHandler(typt, kvs)
		case ssa.UndefinedTypeKind:
			obj = b.InterfaceAddFieldBuild(0,
				func(i int) ssa.Value {
					return b.EmitConstInst(i)
				},
				func(i int) ssa.Value {
					return b.EmitConstInst(i)
				})
		case ssa.NumberTypeKind, ssa.StringTypeKind, ssa.BooleanTypeKind:
			return kvs[0].value
		default:
			if kvs[0].value != nil {
				return kvs[0].value
			}
			b.NewError(ssa.Error, TAG, "unhandled type")
			return b.EmitConstInst(0)
		}
		coverType(obj.GetType(), typ)
		return obj
	}

	rvalue := typeHandler(typ, kvs)
	if o, ok := typ.(*ssa.ObjectType); ok {
		// 非指针匿名结构体，需要创建对象
		for n, a := range o.AnonymousField {
			isFind := false
			for k, _ := range rvalue.GetAllMember() {
				if k.String() == n {
					isFind = true
					break
				}
			}
			if !isFind {
				newObject := typeHandler(a, nil)
				variable := b.CreateMemberCallVariable(rvalue, b.EmitConstInst(n))
				b.AssignVariable(variable, newObject)
			}
		}

		bp := b.CreateBlueprint(o.VerboseName)
		// b.AssignVariable(b.CreateVariable(o.VerboseName), rvalue)
		for n, f := range typ.GetMethod() {
			bp.AddMethod(n, f)
		}
		rvalue.SetType(typ)
	}

	return rvalue
}

func (b *astbuilder) buildLiteralValue(exp *gol.LiteralValueContext, iscreate bool) (ret []keyValue) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if list := exp.ElementList(); list != nil {
		for _, e := range list.(*gol.ElementListContext).AllKeyedElement() {
			kv := b.buildKeyedElement(e.(*gol.KeyedElementContext), iscreate)
			ret = append(ret, kv)
		}
	}

	return ret
}

func (b *astbuilder) buildKeyedElement(exp *gol.KeyedElementContext, iscreate bool) (ret keyValue) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()
	var keys ssa.Value
	var kvs []keyValue

	if key := exp.Key(); key != nil {
		keys = b.buildKey(key.(*gol.KeyContext), iscreate)
	}

	if elem := exp.Element(); elem != nil {
		if s, ok := elem.(*gol.ElementContext); ok {
			kvs = b.buildElement(s, iscreate)
		}
	}

	return keyValue{
		key:   keys,
		value: nil,
		kv:    kvs,
	}
}

func (b *astbuilder) buildKey(exp *gol.KeyContext, iscreate bool) ssa.Value {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if e := exp.Expression(); e != nil {
		if iscreate {
			var leftv ssa.Value
			if p := e.(*gol.ExpressionContext).PrimaryExpr(); p != nil {
				if o := p.(*gol.PrimaryExprContext).Operand(); o != nil {
					if n := o.(*gol.OperandContext).OperandName(); n != nil {
						id := n.(*gol.OperandNameContext).IDENTIFIER()
						leftv = b.EmitConstInst(id.GetText())
					}
				}
			}
			return leftv
		} else {
			rightv, _ := b.buildExpression(e.(*gol.ExpressionContext), false)
			return rightv
		}
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
}

func (b *astbuilder) buildElement(exp *gol.ElementContext, iscreate bool) (ret []keyValue) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if e := exp.Expression(); e != nil {
		right, _ := b.buildExpression(e.(*gol.ExpressionContext), false)
		kv := keyValue{
			key:   nil,
			value: right,
			kv:    []keyValue{},
		}
		ret = append(ret, kv)
		return ret
	}

	if e := exp.LiteralValue(); e != nil {
		return b.buildLiteralValue(e.(*gol.LiteralValueContext), iscreate)
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return ret
}

func (b *astbuilder) buildLiteralType(stmt *gol.LiteralTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if name := stmt.TypeName(); name != nil {
		return b.buildTypeName(name.(*gol.TypeNameContext))
	}

	if stmt.ELLIPSIS() != nil {
		return b.buildSliceTypeELiteral(stmt)
	}

	// slice type literal
	if s, ok := stmt.SliceType().(*gol.SliceTypeContext); ok {
		return b.buildSliceTypeLiteral(s)
	}

	// array type literal
	if s, ok := stmt.ArrayType().(*gol.ArrayTypeContext); ok {
		return b.buildArrayTypeLiteral(s)
	}

	// map type literal
	if s, ok := stmt.MapType().(*gol.MapTypeContext); ok {
		return b.buildMapTypeLiteral(s)
	}

	// struct type literal
	if s, ok := stmt.StructType().(*gol.StructTypeContext); ok {
		return b.buildStructTypeLiteral(s)
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeLit(stmt *gol.TypeLitContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	text := stmt.GetText()

	// slice type literal
	if s := stmt.SliceType(); s != nil {
		return b.buildSliceTypeLiteral(s.(*gol.SliceTypeContext))
	}

	// array type literal
	if s := stmt.ArrayType(); s != nil {
		return b.buildArrayTypeLiteral(s.(*gol.ArrayTypeContext))
	}

	// map type literal
	if strings.HasPrefix(text, "map") {
		if s := stmt.MapType(); s != nil {
			return b.buildMapTypeLiteral(s.(*gol.MapTypeContext))
		}
	}

	// struct type literal
	if strings.HasPrefix(text, "struct") {
		if s := stmt.StructType(); s != nil {
			return b.buildStructTypeLiteral(s.(*gol.StructTypeContext))
		}
	}

	// pointer type literal
	if strings.HasPrefix(text, "*") {
		if p := stmt.PointerType(); p != nil {
			if t := p.(*gol.PointerTypeContext).Type_(); t != nil {
				ssatyp := b.buildType(t.(*gol.Type_Context))
				// newtyp := ssa.NewPointerType()
				// newtyp.SetName("Pointer")
				// newtyp.SetFullTypeNames(ssatyp.GetFullTypeNames())
				// return newtyp
				return ssatyp
			}
		}
	}

	// function type literal
	if strings.HasPrefix(text, "func") {
		if s := stmt.FunctionType(); s != nil {
			return b.buildFunctionTypeLiteral(s.(*gol.FunctionTypeContext))
		}
	}

	// interface type literal
	if strings.HasPrefix(text, "interface") {
		if s := stmt.InterfaceType(); s != nil {
			return b.buildInterfaceTypeLiteral(s.(*gol.InterfaceTypeContext))
		}
	}

	// channel type literal
	if strings.HasPrefix(text, "chan") ||
		strings.HasPrefix(text, "<-chan") ||
		strings.HasPrefix(text, "chan<-") {
		if s := stmt.ChannelType(); s != nil {
			return b.buildChanTypeLiteral(s.(*gol.ChannelTypeContext))
		}
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildFunctionTypeLiteral(stmt *gol.FunctionTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if signature := stmt.Signature(); signature != nil {
		paramt, rett := b.buildSignature(signature.(*gol.SignatureContext))
		return ssa.NewFunctionType("", paramt, rett, false)
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildInterfaceTypeLiteral(stmt *gol.InterfaceTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	interfacetyp := ssa.NewInterfaceType("", "")

	for _, t := range stmt.AllTypeElement() {
		ssatyp := b.buildTypeElement(t.(*gol.TypeElementContext))
		switch t := ssatyp.(type) {
		case *ssa.InterfaceType:
			interfacetyp.AddFatherInterfaceType(t)
			for n, m := range t.GetMethod() {
				interfacetyp.AddMethod(n, m)
			}
		case *ssa.ObjectType:
			interfacetyp.AddStructure(t.Name, t)
		}
	}

	for _, f := range stmt.AllMethodSpec() {
		b.buildMethodSpec(f.(*gol.MethodSpecContext), interfacetyp)
	}

	return interfacetyp
}

func (b *astbuilder) buildChanTypeLiteral(stmt *gol.ChannelTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if etyp := stmt.ElementType(); etyp != nil {
		if typ := etyp.(*gol.ElementTypeContext).Type_(); typ != nil {
			ssatyp := b.buildType(typ.(*gol.Type_Context))
			return ssa.NewChanType(ssatyp)
		}
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildMapTypeLiteral(stmt *gol.MapTypeContext) ssa.Type {
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

	b.NewError(ssa.Error, TAG, Unreachable())
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildSliceTypeLiteral(stmt *gol.SliceTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var ssatyp ssa.Type
	if stmt.GetText() == "[]byte" || stmt.GetText() == "[]uint8" {
		return ssa.CreateBytesType()
	}
	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		if eleTyp := b.buildType(s.Type_().(*gol.Type_Context)); eleTyp != nil {
			ssatyp = ssa.NewSliceType(eleTyp)
		}
	}
	return ssatyp
}

func (b *astbuilder) buildSliceTypeELiteral(stmt *gol.LiteralTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var ssatyp ssa.Type
	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		if eleTyp := b.buildType(s.Type_().(*gol.Type_Context)); eleTyp != nil {
			ssatyp = ssa.NewSliceType(eleTyp)
		}
	}
	return ssatyp
}

func (b *astbuilder) buildArrayTypeLiteral(stmt *gol.ArrayTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var value ssa.Value
	var ssatyp ssa.Type
	if s, ok := stmt.ArrayLength().(*gol.ArrayLengthContext); ok {
		if e := s.Expression(); e != nil {
			rightv, _ := b.buildExpression(e.(*gol.ExpressionContext), false)
			value = rightv
		}
	}

	if s, ok := stmt.ElementType().(*gol.ElementTypeContext); ok {
		if eleTyp := b.buildType(s.Type_().(*gol.Type_Context)); eleTyp != nil {
			ssatyp = ssa.NewSliceType(eleTyp)
		}
	}
	_ = value
	return ssatyp
}

func (b *astbuilder) buildStructTypeLiteral(stmt *gol.StructTypeContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	structTyp := ssa.NewStructType()
	for _, s := range stmt.AllFieldDecl() {
		b.buildFieldDecl(s.(*gol.FieldDeclContext), structTyp)
	}
	return structTyp
}

func (b *astbuilder) buildFieldDecl(stmt *gol.FieldDeclContext, structTyp *ssa.ObjectType) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var ssatyp ssa.Type = nil
	if typ := stmt.Type_(); typ != nil {
		ssatyp = b.buildType(typ.(*gol.Type_Context))
	}

	if idlist := stmt.IdentifierList(); idlist != nil {
		sList := b.buildStructList(idlist.(*gol.IdentifierListContext))
		if ssatyp != nil {
			for _, p := range sList {
				structTyp.AddField(p, ssatyp)
			}
		}
	}

	if em := stmt.EmbeddedField(); em != nil {
		if typ, ok := em.(*gol.EmbeddedFieldContext); ok {
			parent := b.buildTypeName(typ.TypeName().(*gol.TypeNameContext))
			if a := typ.TypeArgs(); a != nil {
				b.tpHandler[b.Function.GetName()] = b.buildTypeArgs(a.(*gol.TypeArgsContext))
			}

			if fromUser, ok := parent.(*ssa.ObjectType); ok {
				structTyp.AnonymousField[typ.TypeName().GetText()] = fromUser
				structTyp.AddField(b.EmitConstInst(typ.TypeName().GetText()), fromUser)
			} else if fromAlias, ok := parent.(*ssa.AliasType); ok {
				if fromUser, ok := ssa.ToObjectType(fromAlias.GetType()); ok {
					structTyp.AnonymousField[typ.TypeName().GetText()] = fromUser
					structTyp.AddField(b.EmitConstInst(typ.TypeName().GetText()), fromUser)
				}
				structTyp.AddField(b.EmitConstInst(fromAlias.Name), fromAlias.GetType())
			} else if fromLib, ok := parent.(*ssa.Blueprint); ok {
				structTyp.AddField(b.EmitConstInst(fromLib.Name), fromLib)
				for _, fn := range fromLib.GetFullTypeNames() {
					structTyp.AddFullTypeName(fn)
				}
			} else if notUse, ok := parent.(*ssa.BasicType); ok {
				// b.NewError(ssa.Warn, TAG, Unreachable())
				structTyp.AddField(b.EmitConstInst(notUse.GetName()), notUse)
			}
		}
	}
}

func (b *astbuilder) buildBasicLit(exp *gol.BasicLitContext) ssa.Value {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if lit := exp.Integer(); lit != nil {
		return b.buildIntegerLiteral(lit.(*gol.IntegerContext))
	}

	if lit := exp.NIL_LIT(); lit != nil {
		return b.EmitConstInstNil()
	}

	if lit := exp.FLOAT_LIT(); lit != nil {
		t := lit.GetText()
		if strings.HasPrefix(t, ".") {
			t = "0" + t
		}
		f, _ := strconv.ParseFloat(t, 64)
		return b.EmitConstInst(f)
	}

	if lit := exp.String_(); lit != nil {
		return b.buildStringLiteral(lit.(*gol.String_Context))
	}

	if lit := exp.Char_(); lit != nil {
		return b.buildCharLiteral(lit.(*gol.Char_Context))
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
}

func (b *astbuilder) buildStringLiteral(stmt *gol.String_Context) ssa.Value {
	text := stmt.GetText()
	if text == "" {
		return b.EmitConstInst(text)
	}

	switch text[0] {
	case '"':
		return b.EmitConstInstWithUnary(text[1:len(text)-1], 0)
	case '`':
		return b.EmitConstInstWithUnary(text[1:len(text)-1], 0)
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0)
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
			b.NewError(ssa.Error, TAG, fmt.Sprintf("unquote error %s", err))
			return b.EmitConstInst(0)
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
		f, _ := strconv.ParseFloat(lit, 64)
		return b.EmitConstInst(f)
	} else {
		var err error
		originStr := stmt.GetText()
		intStr := strings.ToLower(originStr)
		var resultInt64 int64

		if num := stmt.DECIMAL_LIT(); num != nil { // 十进制
			if strings.Contains(stmt.GetText(), "e") {
				f, _ := strconv.ParseFloat(intStr, 64)
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
			b.NewError(ssa.Error, TAG, fmt.Sprintf("cannot parse num for literal: %s", stmt.GetText()))
			return b.EmitConstInst(0)
		}

		if err != nil {
			b.NewError(ssa.Error, TAG, fmt.Sprintf("const parse %s as integer literal... is to large for int64: %v", originStr, err))
			return b.EmitConstInst(0)
		}

		if resultInt64 > math.MaxInt {
			return b.EmitConstInst(int64(resultInt64))
		} else {
			return b.EmitConstInst(int64(resultInt64))
		}
	}
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
	case ssa.StructTypeKind:
		typ.FieldType = wantTyp.FieldType
		typ.KeyTyp = wantTyp.KeyTyp
		wantTyp.RangeMethod(func(s string, f *ssa.Function) {
			typ.AddMethod(s, f)
		})
	}
	for n, a := range wantTyp.AnonymousField {
		// TODO: 匿名结构体可能是一个指针，修改时应该要连带父类一起修改
		typ.AnonymousField[n] = a
	}
}

func (b *astbuilder) GetDefaultValue(ityp ssa.Type) ssa.Value {
	switch ityp.GetTypeKind() {
	case ssa.NumberTypeKind:
		return b.EmitConstInst(0)
	case ssa.StringTypeKind:
		return b.EmitConstInst("")
	case ssa.BooleanTypeKind:
		return b.EmitConstInst(false)
	case ssa.FunctionTypeKind:
		return b.EmitUndefined("func")
	case ssa.AliasTypeKind:
		alias, _ := ssa.ToAliasType(ityp)
		return b.GetDefaultValue(alias.GetType())
	case ssa.StructTypeKind, ssa.ObjectTypeKind, ssa.InterfaceTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind:
		return b.EmitMakeBuildWithType(ityp, nil, nil)
	default:
		return b.EmitConstInst(0)
	}
}
