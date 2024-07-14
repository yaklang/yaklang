package php2ssa

import (
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/utils"
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
	if i.AnonymousClass() != nil {
		return y.VisitAnonymousClass(i.AnonymousClass())
	}
	className := i.TypeRef().GetText()
	class := y.GetClassBluePrint(className)
	obj := y.EmitMakeWithoutType(nil, nil)
	if class == nil {
		log.Warnf("class %v instantiation failed, checking the dependency package is loaded already?", className)
		obj.SetType(ssa.GetAnyType())
		return obj
	}
	obj.SetType(class)

	findConstructorAndDestruct := func(class *ssa.ClassBluePrint) (ssa.Value, ssa.Value) {
		tmpClass := class
		var (
			constructor ssa.Value = nil
			destructor  ssa.Value = nil
		)

		for {
			if tmpClass.Constructor != nil && constructor == nil {
				constructor = tmpClass.Constructor
			}
			if tmpClass.Destructor != nil && destructor == nil {
				destructor = tmpClass.Destructor
			}
			if constructor != nil && destructor != nil {
				return constructor, destructor
			}
			if len(tmpClass.ParentClass) != 0 {
				tmpClass = class.ParentClass[0]
			} else {
				return constructor, destructor
			}
		}
	}
	args := []ssa.Value{obj}
	constructor, destructor := findConstructorAndDestruct(class)
	if destructor != nil {
		call := y.NewCall(destructor, args)
		y.AddDefer(call)
	}
	if constructor == nil {
		return obj
	}
	ellipsis := false
	if i.Arguments() != nil {
		tmp, hasEllipsis := y.VisitArguments(i.Arguments())
		ellipsis = hasEllipsis
		args = append(args, tmp...)
	}
	c := y.NewCall(constructor, args)
	c.IsEllipsis = ellipsis
	y.EmitCall(c)

	return obj
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
			className = y.VisitIdentifier(i.Identifier())

			parentClassName := ""
			if i.Extends() != nil {
				parentClassName = i.QualifiedStaticTypeRef().GetText()
			}

			class := y.CreateClassBluePrint(className)
			if parentClass := y.GetClassBluePrint(parentClassName); parentClass != nil {
				//感觉在ssa-classBlue中做更好，暂时修复
				class.AddParentClass(parentClass)
			}
			for _, statement := range i.AllClassStatement() {
				y.VisitClassStatement(statement, class)
			}
		}
	} else {
		// as interface
		className = y.VisitIdentifier(i.Identifier())
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

	switch ret := raw.(type) {
	case *phpparser.PropertyModifiersVariableContext:
		// variable
		modifiers := y.VisitPropertyModifiers(ret.PropertyModifiers())
		// handle type hint
		typ := y.VisitTypeHint(ret.TypeHint())

		setMember := func(name string, value ssa.Value) {
			_, isStatic := modifiers[ssa.Static]
			isNilValue := utils.IsNil(value)

			switch {
			case isStatic && isNilValue:
				// static member only type
			case isStatic && !isNilValue:
				// static member
				variable := y.GetStaticMember(class.Name, name)
				y.AssignVariable(variable, value)
				class.AddStaticMember(name, value)
			case !isStatic && isNilValue:
				// normal member only type
				class.AddNormalMemberOnlyType(name, typ)
			case !isStatic && !isNilValue:
				// normal member
				class.AddNormalMember(name, value)
			}
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
		isStatic := false
		if _, ok := memberModifiers[ssa.Static]; ok {
			isStatic = true
		}

		isRef := ret.Ampersand()
		_ = isRef

		funcName := y.VisitIdentifier(ret.Identifier())

		createFunction := func() *ssa.Function {
			newFunction := y.NewFunc(funcName)
			y.FunctionBuilder = y.PushFunction(newFunction)
			{
				this := y.NewParam("$this")
				this.SetType(class)
				y.VisitFormalParameterList(ret.FormalParameterList())
				y.VisitMethodBody(ret.MethodBody())
				y.Finish()
			}
			y.FunctionBuilder = y.PopFunction()
			return newFunction
		}

		switch funcName {
		case "__construct":
			newFunction := createFunction()
			class.Constructor = newFunction
		case "__destruct":
			function := createFunction()
			class.Destructor = function
		default:
			newFunction := createFunction()
			if isStatic {
				variable := y.GetStaticMember(class.Name, newFunction.GetName())
				y.AssignVariable(variable, newFunction)
				class.AddStaticMethod(funcName, newFunction)
			} else {
				class.AddMethod(funcName, newFunction)
			}
		}
	case *phpparser.ConstContext:
		// TODO: ret.Attributes() // php8
		memberModifiers := y.VisitMemberModifiers(ret.MemberModifiers())
		_ = memberModifiers
		// handle type hint
		// typ := y.VisitTypeHint(ret.TypeHint())

		for _, init := range ret.AllIdentifierInitializer() {
			name, value := y.VisitIdentifierInitializer(init)
			if value == nil && name == "" {
				log.Errorf("const %v has been defined value is %v", name, value)
				continue
			}
			y.AssignClassConst(class.Name, name, value)
		}

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
		y.NewError(ssa.Warn, "trait.adaptation", "unknown trait adaptation statement")
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
		y.BuildSyntaxBlock(func() {
			y.VisitBlockStatement(i.BlockStatement())
		})
	}

	return nil
}

func (y *builder) VisitIdentifierInitializer(raw phpparser.IIdentifierInitializerContext) (string, ssa.Value) {
	if y == nil || raw == nil {
		return "", nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.IdentifierInitializerContext)
	if i == nil {
		return "", nil
	}
	var unquote string
	rawName := y.VisitIdentifier(i.Identifier())
	_unquote, err := yakunquote.Unquote(rawName)
	if err != nil {
		unquote = rawName
	} else {
		unquote = _unquote
	}

	return unquote, y.VisitConstantInitializer(i.ConstantInitializer())
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

	name := i.VarName().GetText()
	var val ssa.Value
	if constInit := i.ConstantInitializer(); constInit != nil {
		val = y.VisitConstantInitializer(i.ConstantInitializer())
	}
	// if utils.IsNil(val) {
	// 	undefined := y.EmitUndefined(name)
	// 	undefined.Kind = ssa.UndefinedMemberValid
	// 	val = undefined
	// }
	return name, val
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

func (y *builder) VisitStaticClassExprFunctionMember(raw phpparser.IStaticClassExprFunctionMemberContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	getValue := func(class, key string) ssa.Value {
		// class const  variable
		if !y.isFunction {
			if v, ok := y.ReadClassConst(class, key); ok {
				return v
			}
		}
		// function
		variable := y.GetStaticMember(class, key)
		return y.ReadValueByVariable(variable)
	}

	// var class, key string
	switch i := raw.(type) {
	case *phpparser.ClassStaticFunctionMemberContext:
		// TODO: class
		key := i.Identifier().GetText()
		_ = key
	case *phpparser.ClassDirectFunctionMemberContext:
		class := i.Identifier(0).GetText()
		key := i.Identifier(1).GetText()
		return getValue(class, key)
	case *phpparser.StringAsIndirectClassStaticFunctionMemberContext:
		class := ""
		str, err := strconv.Unquote(i.String_().GetText())
		if err != nil {
			class = i.String_().GetText()
		} else {
			class = str
		}
		key := i.Identifier().GetText()
		return getValue(class, key)
	case *phpparser.VariableAsIndirectClassStaticFunctionMemberContext:
		exprName := y.VisitVariable(i.Variable())
		value := y.ReadValue(exprName)
		key := i.Identifier().GetText()
		// if value is instance of class, check this class static function or const member
		if typ, ok := ssa.ToObjectType(value.GetType()); ok {
			if v := getValue(typ.Name, key); v != nil {
				return v
			}
		}
		return getValue(value.String(), key)
	default:
		_ = i
	}
	return nil
}

func (y *builder) VisitStaticClassExprVariableMember(raw phpparser.IStaticClassExprVariableMemberContext) *ssa.Variable {
	if y == nil || raw == nil {
		return nil
	}
	var class, key string
	switch i := raw.(type) {
	case *phpparser.ClassStaticVariableContext:
		// TODO class 命令空间
		//key = i.VarName().GetText()
	case *phpparser.ClassDirectStaticVariableContext:
		//肯定是一个class，
		class = i.Identifier().GetText()
		key = y.VisitRightValue(i.FlexiVariable()).GetName()
		//key = i.VarName().GetText()
	case *phpparser.StringAsIndirectClassStaticVariableContext:
		// "test"::a;
		str, err := strconv.Unquote(i.String_().GetText())
		if err != nil {
			class = i.String_().GetText()
		} else {
			class = str
		}
		key = y.VisitRightValue(i.FlexiVariable()).GetName()
	case *phpparser.VariableAsIndirectClassStaticVariableContext:
		exprName := y.VisitVariable(i.Variable())
		class = y.ReadValue(exprName).String()
		key = y.VisitRightValue(i.FlexiVariable()).GetName()
		//return y.GetStaticMember(class, value.String())
	default:
		_ = i
	}
	if class == "" {
		return nil
	}
	if strings.HasPrefix(key, "$") {
		// variable
		key = key[1:]
		return y.GetStaticMember(class, key)
	}
	// function
	return y.GetStaticMember(class, key)
}

func (y *builder) VisitStaticClassExpr(raw phpparser.IStaticClassExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	if i, ok := raw.(*phpparser.StaticClassExprContext); ok {
		if i.StaticClassExprFunctionMember() != nil {
			return y.VisitStaticClassExprFunctionMember(i.StaticClassExprFunctionMember())
		}
		if i.StaticClassExprVariableMember() != nil {
			variable := y.VisitStaticClassExprVariableMember(i.StaticClassExprVariableMember())
			return y.ReadValueByVariable(variable)
		}
	}

	return nil
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
		return y.EmitConstInst(i.Identifier().GetText())
	}

	if i.Variable() != nil {
		name := y.VisitVariable(i.Variable())
		value := y.ReadValue(name)
		return y.EmitConstInst(value.String())
	}

	if i.String_() != nil {
		return y.EmitConstInst(i.String_().GetText())
	}

	if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	}

	return y.EmitUndefined(raw.GetText())
}

func (y *builder) VisitAnonymousClass(raw phpparser.IAnonymousClassContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AnonymousClassContext)
	if i == nil {
		return nil
	}
	cname := uuid.NewString()
	bluePrint := y.CreateClassBluePrint(cname)
	if i.QualifiedStaticTypeRef() != nil {
		ref := y.VisitQualifiedStaticTypeRef(i.QualifiedStaticTypeRef())
		if classBluePrint := y.GetClassBluePrint(ref); classBluePrint != nil {
			bluePrint.AddParentClass(classBluePrint)
		}
	}
	for _, statement := range i.AllClassStatement() {
		y.VisitClassStatement(statement, bluePrint)
	}
	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(bluePrint)
	constructor := bluePrint.Constructor
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
	c := y.NewCall(constructor, args)
	c.IsEllipsis = ellipsis
	y.EmitCall(c)
	return obj
}
