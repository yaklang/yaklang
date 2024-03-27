package php2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitNewExpr(raw phpparser.INewExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.NewExprContext)
	if i == nil {
		return nil
	}
	className := i.TypeRef().GetText()
	class := y.ir.GetClass(className)
	obj := y.ir.EmitMakeWithoutType(nil, nil)
	obj.SetType(class)

	constructor := class.Constructor
	if constructor == nil {
		return obj
	}

	args := []ssa.Value{obj}
	ellipsis := false
	if i.Arguments() != nil {
		tmp, hasEllipsis := y.VisitArguments(i.Arguments())
		ellipsis = hasEllipsis
		args = append(args, tmp...)
	}
	c := y.ir.NewCall(constructor, args)
	c.IsEllipsis = ellipsis
	y.ir.EmitCall(c)
	return obj
}

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeRefContext)
	if i == nil {
		return nil
	}

	if i.QualifiedNamespaceName() != nil {
		y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	} else if i.IndirectTypeRef() != nil {

	} else if i.PrimitiveType() != nil {
		return y.VisitPrimitiveType(i.PrimitiveType())
	} else if i.Static() != nil {
		// as class name
	}

	return nil
}

func (y *builder) VisitPrimitiveType(raw phpparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}

	if i.BoolType() != nil {
		return ssa.GetTypeByStr("bool")
	} else if i.IntType() != nil {
		return ssa.GetTypeByStr("int")
	} else if i.Int64Type() != nil {
		return ssa.GetTypeByStr("int64")
	} else if i.DoubleType() != nil {
		return ssa.GetTypeByStr("float64")
	} else if i.StringType() != nil {
		return ssa.GetTypeByStr("string")
	} else if i.Resource() != nil {
		return ssa.GetTypeByStr("any")
	} else if i.ObjectType() != nil {
		return ssa.GetTypeByStr("any")
	} else if i.Array() != nil {
		return ssa.NewMapType(ssa.GetTypeByStr("any"), ssa.GetTypeByStr("any"))
	}
	return ssa.GetTypeByStr("any")
}

func (y *builder) VisitClassDeclaration(raw phpparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ClassDeclarationContext)
	if i == nil {
		return nil
	}

	// notes #[...] for dec
	if i.Attributes() != nil {
	}
	// access / private?
	if i.Private() != nil {
		// handle priv
		// not right for class, u can save as an abnormal!
	}

	// modifier: final / abstract
	if i.Modifier() != nil {
		// handle modifier
	}

	if i.Partial() != nil {
		// not in PHP, as abnormal
	}

	var className string
	var mergedTemplate []string
	if i.ClassEntryType() != nil {
		switch strings.ToLower(i.ClassEntryType().GetText()) {
		case "trait":
			// trait class is not allowed to be inherited / extend / impl
			// as class alias is right as compiler! XD
			fallthrough
		case "class":
			className = i.Identifier().GetText()

			parentClassName := ""
			if i.Extends() != nil {
				parentClassName = i.QualifiedStaticTypeRef().GetText()
			}

			class := y.ir.CreateClass(className)
			if parentClass := y.ir.GetClass(parentClassName); parentClass != nil {
				class.AddParentClass(parentClass)
			}
			for _, statement := range i.AllClassStatement() {
				y.VisitClassStatement(statement, class)
			}
		}
	} else {
		// as interface
		className = i.Identifier().GetText()
		if i.Extends() != nil {
			for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
				mergedTemplate = append(mergedTemplate, impl.GetText())
			}
		}
	}

	return nil
}

func (y *builder) VisitClassStatement(raw phpparser.IClassStatementContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
		return
	}

	// i, _ := raw.(*phpparser.ClassStatementContext)
	// if i == nil {
	// 	return
	// }

	switch ret := raw.(type) {
	case *phpparser.PropertyModifiersVariableContext:
		// variable
		modifiers := y.VisitPropertyModifiers(ret.PropertyModifiers())

		setMember := class.AddNormalMember
		if _, ok := modifiers[ssa.Static]; ok {
			setMember = func(name string, value ssa.Value) {
				variable := y.ir.GetStaticMember(class.Name, name)
				y.ir.AssignVariable(variable, value)
				class.AddStaticMember(name, value)
			}
		}
		// handle variable
		if ret.TypeHint() != nil {
			// handle type hint
			y.VisitTypeHint(ret.TypeHint())
		}

		// handle variable name
		for _, va := range ret.AllVariableInitializer() {
			name, value := y.VisitVariableInitializer(va)
			if strings.HasPrefix(name, "$") {
				name = name[1:]
			}
			setMember(name, value)
		}

		return
	case *phpparser.FunctionContext:
		// function
		// TODO: ret.Attributes() // php8
		memberModifiers := y.VisitMemberModifiers(ret.MemberModifiers())
		_ = memberModifiers
		isRef := ret.Ampersand()
		_ = isRef

		funcName := ret.Identifier().GetText()

		createFunction := func() *ssa.Function {
			newFunction := y.ir.NewFunc(funcName)
			y.ir = y.ir.PushFunction(newFunction)
			{
				this := y.ir.NewParam("$this")
				_ = this
				y.VisitFormalParameterList(ret.FormalParameterList())
				y.VisitMethodBody(ret.MethodBody())
				y.ir.Finish()
			}
			y.ir = y.ir.PopFunction()
			return newFunction
		}

		switch funcName {
		case "__construct":
			newFunction := createFunction()
			class.Constructor = newFunction
		default:
			newFunction := createFunction()
			class.AddNormalMethod(funcName, newFunction, 0)
		}
	case *phpparser.ConstContext:
		// TODO: ret.Attributes() // php8
		memberModifiers := y.VisitMemberModifiers(ret.MemberModifiers())
		_ = memberModifiers
	case *phpparser.TraitUseContext:
	default:

	}
	return
}

func (y *builder) VisitTraitAdaptations(raw phpparser.ITraitAdaptationsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TraitAdaptationsContext)
	if i == nil {
		return nil
	}

	for _, t := range i.AllTraitAdaptationStatement() {
		y.VisitTraitAdaptationStatement(t)
	}

	return nil
}

func (y *builder) VisitTraitAdaptationStatement(raw phpparser.ITraitAdaptationStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TraitAdaptationStatementContext)
	if i == nil {
		return nil
	}

	if i.TraitPrecedence() != nil {
		y.VisitTraitPrecedence(i.TraitPrecedence())
	} else if i.TraitAlias() != nil {
		// trait alias
		y.VisitTraitAlias(i.TraitAlias())
	} else {
		y.ir.NewError(ssa.Warn, "trait.adaptation", "unknown trait adaptation statement")
	}

	return nil
}

func (y *builder) VisitTraitAlias(raw phpparser.ITraitAliasContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TraitAliasContext)
	if i == nil {
		return nil
	}

	i.TraitMethodReference()
	if i.Identifier() != nil {
		// memberModifier
		y.VisitIdentifier(i.Identifier())
	} else {
		idStr := i.MemberModifier().GetText()
		_ = idStr
	}

	return nil
}

func (y *builder) VisitTraitPrecedence(raw phpparser.ITraitPrecedenceContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TraitPrecedenceContext)
	if i == nil {
		return nil
	}

	y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	y.VisitIdentifier(i.Identifier())
	y.VisitQualifiedNamespaceNameList(i.QualifiedNamespaceNameList())

	return nil
}

func (y *builder) VisitMethodBody(raw phpparser.IMethodBodyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.MethodBodyContext)
	if i.BlockStatement() != nil {
		y.ir.BuildSyntaxBlock(func() {
			y.VisitBlockStatement(i.BlockStatement())
		})
	}

	return nil
}

func (y *builder) VisitIdentifierInitializer(raw phpparser.IIdentifierInitializerContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.IdentifierInitializerContext)
	if i == nil {
		return nil
	}
	var unquote string
	_unquote, err := yakunquote.Unquote(i.Identifier().GetText())
	if err != nil {
		unquote = i.Identifier().GetText()
	} else {
		unquote = _unquote
	}
	if ConstValue, ok := y.constMap[unquote]; ok {
		log.Warnf("const %v has been defined value is %v", unquote, ConstValue.String())
	} else {
		//y.ir.AssignVariable(y.ir.CreateVariable(i.Identifier().GetText()), y.VisitConstantInitializer(i.ConstantInitializer()))
		y.constMap[unquote] = y.VisitConstantInitializer(i.ConstantInitializer())
	}
	return nil
}

// VisitVariableInitializer read ast and return varName and ssaValue
func (y *builder) VisitVariableInitializer(raw phpparser.IVariableInitializerContext) (string, ssa.Value) {
	if y == nil || raw == nil {
		return "", nil
	}

	i, _ := raw.(*phpparser.VariableInitializerContext)
	if i == nil {
		return "", nil
	}

	var val ssa.Value
	if constInit := i.ConstantInitializer(); constInit != nil {
		val = y.VisitConstantInitializer(i.ConstantInitializer())
	} else {
		val = y.ir.EmitConstInstAny()
	}
	return i.VarName().GetText(), val
}

func (y *builder) VisitClassConstant(raw phpparser.IClassConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ClassConstantContext)
	if i == nil {
		return nil
	}

	panic("CLASS CONSTANT TODO")

	return nil
}

func (y *builder) VisitStaticClassExpr(raw phpparser.IStaticClassExprContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	var class, key string
	switch i := raw.(type) {
	case *phpparser.ClassStaticFunctionMemberContext:
		// TODO: class
		key = i.Identifier().GetText()
	case *phpparser.ClassStaticVariableContext:
		// TODO class
		key = i.VarName().GetText()
	case *phpparser.ClassDirectFunctionMemberContext:
		class = i.Identifier(0).GetText()
		key = i.Identifier(1).GetText()
	case *phpparser.ClassDirectStaticVariableContext:
		class = i.Identifier().GetText()
		key = i.VarName().GetText()
	case *phpparser.StringAsIndirectClassStaticFunctionMemberContext:
		class = i.String_().GetText()
		key = i.Identifier().GetText()
	case *phpparser.StringAsIndirectClassStaticVariableContext:
		class = i.String_().GetText()
		key = i.VarName().GetText()
	default:
		_ = i
	}
	if strings.HasPrefix(key, "$") {
		key = key[1:]
	}

	return y.ir.GetStaticMember(class, key)
}

/// class modifier

func (y *builder) VisitPropertyModifiers(raw phpparser.IPropertyModifiersContext) map[ssa.ClassModifier]struct{} {
	ret := make(map[ssa.ClassModifier]struct{})
	i, ok := raw.(*phpparser.PropertyModifiersContext)
	if !ok {
		return ret
	}

	if i.Var() != nil {
		return ret
	} else {
		return y.VisitMemberModifiers(i.MemberModifiers())
	}
}

func (y *builder) VisitMemberModifiers(raw phpparser.IMemberModifiersContext) map[ssa.ClassModifier]struct{} {
	ret := make(map[ssa.ClassModifier]struct{})
	i, ok := raw.(*phpparser.MemberModifiersContext)
	if !ok {
		return ret
	}

	for _, item := range i.AllMemberModifier() {
		ret[y.VisitMemberModifier(item)] = struct{}{}
	}

	return ret
}

func (y *builder) VisitMemberModifier(raw phpparser.IMemberModifierContext) ssa.ClassModifier {
	i, ok := raw.(*phpparser.MemberModifierContext)
	if !ok {
		return ssa.NoneModifier
	}

	if i.Public() != nil {
		return ssa.Public
	} else if i.Protected() != nil {
		return ssa.Protected
	} else if i.Private() != nil {
		return ssa.Private
	} else if i.Static() != nil {
		return ssa.Static
	} else if i.Final() != nil {
		return ssa.Final
	} else if i.Abstract() != nil {
		return ssa.Abstract
	} else if i.Readonly() != nil {
		return ssa.Readonly
	} else {
		return ssa.NoneModifier
	}
}

func (y *builder) VisitIndexMemberCallKey(raw phpparser.IIndexMemberCallKeyContext) ssa.Value {
	i, ok := raw.(*phpparser.IndexMemberCallKeyContext)

	if !ok {
		return nil
	}

	if i.NumericConstant() != nil {
		return y.VisitNumericConstant(i.NumericConstant())
	}

	if i.MemberCallKey() != nil {
		return y.VisitMemberCallKey(i.MemberCallKey())
	}

	//冗余 后续修改
	if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	}
	return nil

}

func (y *builder) VisitMemberCallKey(raw phpparser.IMemberCallKeyContext) ssa.Value {
	i, ok := raw.(*phpparser.MemberCallKeyContext)
	if !ok {
		return nil
	}

	_ = i
	if i.Identifier() != nil {
		return y.ir.EmitConstInst(i.Identifier().GetText())
	}

	if i.Variable() != nil {
		name := y.VisitVariable(i.Variable())
		value := y.ir.ReadValue(name)
		return y.ir.EmitConstInst(value.String())
	}

	if i.String_() != nil {
		return y.ir.EmitConstInst(i.String_().GetText())
	}

	return nil
}
