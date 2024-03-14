package php2ssa

import (
	"strings"

	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitNewExpr(raw phpparser.INewExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.NewExprContext)
	if i == nil {
		return nil
	}

	t := y.VisitTypeRef(i.TypeRef())
	_ = t
	if i.Arguments() != nil {
		return nil
	}
	return nil
}

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

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

			if i.Extends() != nil {
				mergedTemplate = append(mergedTemplate, i.QualifiedStaticTypeRef().GetText())
			}

			if i.Implements() != nil {
				for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
					mergedTemplate = append(mergedTemplate, impl.GetText())
				}
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
	_ = className
	_ = mergedTemplate

	//// how to build a template?
	//// y.ir is a SSA.Function
	class := y.ir.CreateClass(className)
	for _, statement := range i.AllClassStatement() {
		y.VisitClassStatement(statement, class)

	}

	// class.AddMethod()
	//template := y.ir.BuildObjectTemplate(objectTemplate)    // 注册一个对象模版（有构造和析构方法的对象）
	//template.SetDecorationVerbose(...)                        // 记录一下修饰词
	//for _, i := range mergedTemplate {
	//	y.ir.FindObjectTemplate(i).MergeTo(template)        // 合并模版（inherit / trait / extend 都一样）
	//}
	//template.BuildField(func() {                              // 编译字段
	//	for _, field := range i.AllClassStatement() {
	//		y.VisitClassStatement(field)
	//	}
	//})
	//template.Finish()                                         // 宣告完成

	return nil
}

func (y *builder) VisitClassStatement(raw phpparser.IClassStatementContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
		return
	}

	i, _ := raw.(*phpparser.ClassStatementContext)
	if i == nil {
		return
	}

	// note: PHP8 #[...] attributes
	if i.Attributes() != nil {
		// handle php8
	}

	var memberDecorationVerbose string
	if modifiers := i.PropertyModifiers(); modifiers != nil { // variable
		_ = modifiers
		modifiers := y.VisitPropertyModifiers(i.PropertyModifiers())

		setMember := class.BuildMember
		if _, ok := modifiers[ssa.Static]; ok {
			setMember = class.BuildStaticMember
		}

		// handle variable
		if i.TypeHint() != nil {
			// handle type hint
			y.VisitTypeHint(i.TypeHint())
		}

		// handle variable name
		for _, va := range i.AllVariableInitializer() {
			name, value := y.VisitVariableInitializer(va)
			setMember(name, value)
		}

		return
	} else if i.MemberModifiers() != nil {
		memberDecorationVerbose = i.MemberModifiers().GetText()
		// const / function
		if i.Const() != nil {
			// handle const
			if i.TypeHint() != nil {
				varType := y.VisitTypeHint(i.TypeHint())
				_ = varType
			}
			for _, c := range i.AllIdentifierInitializer() {
				y.VisitIdentifierInitializer(c)
			}
		} else if i.Function_() != nil {
			isFuncRef := i.Ampersand() != nil
			var funcName = i.Identifier()
			if i.FormalParameterList() != nil {
				// handle formal parameter list
				y.VisitFormalParameterList(i.FormalParameterList())
			}
			_, _ = isFuncRef, funcName

			// baseCtorCall
			if i.BaseCtorCall() != nil {
				// handle base ctor call
				y.VisitBaseCtorCall(i.BaseCtorCall())
			} else if i.ReturnTypeDecl() != nil {
				// handle return type decl
				y.VisitReturnTypeDecl(i.ReturnTypeDecl())
			}

			y.VisitMethodBody(i.MethodBody())
		}
	} else if i.Use() != nil {
		y.VisitQualifiedNamespaceNameList(i.QualifiedNamespaceNameList())
		y.VisitTraitAdaptations(i.TraitAdaptations())
	}
	_ = memberDecorationVerbose

	return
}

func (y *builder) VisitTraitAdaptations(raw phpparser.ITraitAdaptationsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*phpparser.MethodBodyContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitReturnTypeDecl(raw phpparser.IReturnTypeDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ReturnTypeDeclContext)
	if i == nil {
		return nil
	}

	allowNull := i.QuestionMark() != nil
	t := y.VisitTypeHint(i.TypeHint())
	_ = allowNull
	// t.Union(Null)

	return t
}

func (y *builder) VisitBaseCtorCall(raw phpparser.IBaseCtorCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.BaseCtorCallContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitFormalParameterList(raw phpparser.IFormalParameterListContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FormalParameterListContext)
	if i == nil {
		return nil
	}

	for _, param := range i.AllFormalParameter() {
		y.VisitFormalParameter(param)
	}

	return nil
}

func (y *builder) VisitFormalParameter(raw phpparser.IFormalParameterContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FormalParameterContext)
	if i == nil {
		return nil
	}

	// PHP8 annotation
	if i.Attributes() != nil {
		_ = i.Attributes().GetText()
	}

	// member modifier cannot be used in function formal params
	if len(i.AllMemberModifier()) > 0 {
		// what the fuck?
	}

	allowNull := i.QuestionMark() != nil
	_ = allowNull

	typeHint := y.VisitTypeHint(i.TypeHint())
	isRef := i.Ampersand() != nil
	isVariadic := i.Ellipsis()
	_, _, _ = typeHint, isRef, isVariadic
	formalParams, defaultValue := y.VisitVariableInitializer(i.VariableInitializer())
	param := y.ir.NewParam(formalParams)
	if defaultValue != nil {
		param.SetDefault(defaultValue)
		if t := defaultValue.GetType(); t != nil {
			param.SetType(t)
		}
	}
	if typeHint != nil {
		param.SetType(typeHint)
	}
	return nil
}

func (y *builder) VisitIdentifierInitializer(raw phpparser.IIdentifierInitializerContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.IdentifierInitializerContext)
	if i == nil {
		return nil
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

	i, _ := raw.(*phpparser.ClassConstantContext)
	if i == nil {
		return nil
	}

	panic("CLASS CONSTANT TODO")

	return nil
}

func (y *builder) VisitStaticClassExpr(raw phpparser.IStaticClassExprContext) (string, string) {
	if y == nil || raw == nil {
		return "", ""
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

	return class, key
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
