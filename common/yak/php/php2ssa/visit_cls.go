//go:build !no_language
// +build !no_language

package php2ssa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitNewExpr(raw phpparser.INewExprContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
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
	class, name := y.VisitTypeRef(i.TypeRef())
	var obj ssa.Value
	obj = y.EmitUndefined(name)
	if utils.IsNil(obj) {
		log.Errorf("BUG: container cannot be empty or nil in: %v", raw.GetText())
		log.Errorf("BUG: container cannot be empty or nil in: %v", raw.GetText())
		log.Errorf("BUG: container cannot be empty or nil in: %v", raw.GetText())
		log.Errorf("BUG: container cannot be empty or nil in: %v", raw.GetText())
		return y.EmitUndefined(raw.GetText())
	}
	obj.SetType(class)
	ellipsis := false
	args := []ssa.Value{obj}
	if i.Arguments() != nil {
		tmp, hasEllipsis := y.VisitArguments(i.Arguments())
		ellipsis = hasEllipsis
		args = append(args, tmp...)
	}
	_ = ellipsis
	return y.ClassConstructor(class, args)
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
	// cname := uuid.NewString()

	var (
		interfaces []string
		extendName string
	)
	tokenMap := make(map[string]ssa.CanStartStopToken)

	cname := fmt.Sprintf("anonymous_%s_%s", y.CurrentFile, y.CurrentRange.GetStart())
	blueprint := y.CreateBlueprint(cname)

	if i.ClassEntryType() != nil {
		blueprint.SetKind(ssa.BlueprintClass)
		if i.Extends() != nil {
			if ref := y.VisitQualifiedStaticTypeRef(i.QualifiedStaticTypeRef()); ref != nil {
				extendName = ref.Name
				tokenMap[extendName] = i.QualifiedStaticTypeRef()
			}
		}
		if i.Implements() != nil {
			for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
				interfaces = append(interfaces, impl.GetText())
				tokenMap[impl.GetText()] = impl
			}
		}
	}
	for _, impl := range interfaces {
		bp := y.GetBluePrint(impl)
		if bp == nil {
			bp = y.CreateBlueprint(impl, tokenMap[impl])
		}
		blueprint.AddInterfaceBlueprint(bp)
	}
	if extendName != "" {
		bp := y.GetBluePrint(extendName)
		if bp == nil {
			bp = y.CreateBlueprint(extendName, tokenMap[extendName])
		}
		blueprint.AddParentBlueprint(bp)
	}
	for _, statement := range i.AllClassStatement() {
		y.VisitClassStatement(statement, blueprint)
	}
	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(blueprint)
	args := []ssa.Value{obj}
	ellipsis := false
	if i.Arguments() != nil {
		tmp, hasEllipsis := y.VisitArguments(i.Arguments())
		ellipsis = hasEllipsis
		args = append(args, tmp...)
	}
	_ = ellipsis
	return y.ClassConstructor(blueprint, args)
}

func (y *builder) VisitClassDeclaration(raw phpparser.IClassDeclarationContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ClassDeclarationContext)
	if i == nil {
		return
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

	var (
		blueprint *ssa.Blueprint
	)
	var handlefunc []func(b *builder)

	if i.ClassEntryType() != nil {
		switch strings.ToLower(i.ClassEntryType().GetText()) {
		case "trait":
			// trait class is not allowed to be inherited / extend / impl
			// as class alias is right as compiler! XD
			fallthrough
		case "class":
			name := y.VisitIdentifier(i.Identifier())
			blueprint = y.GetBluePrint(name)
			if blueprint == nil {
				blueprint = y.CreateBlueprint(name, i.Identifier())
			}
			blueprint.SetKind(ssa.BlueprintClass)
			y.MarkedThisClassBlueprint = blueprint
			defer func() {
				y.MarkedThisClassBlueprint = nil
			}()
			y.GetProgram().SetExportType(name, blueprint)
			if i.Extends() != nil {
				handlefunc = append(handlefunc, func(b *builder) {
					b.SetRange(i.QualifiedStaticTypeRef())
					extendBlueprint := y.VisitQualifiedStaticTypeRef(i.QualifiedStaticTypeRef())
					blueprint.AddParentBlueprint(extendBlueprint)
				})
			}
			if i.Implements() != nil {
				handlefunc = append(handlefunc, func(b *builder) {
					for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
						ref := impl.(*phpparser.QualifiedStaticTypeRefContext)
						pkgName, namespaceName := b.VisitQualifiedNamespaceName(ref.QualifiedNamespaceName())
						app := b.GetProgram().GetApplication()
						var iface *ssa.Blueprint
						if len(pkgName) <= 1 {
							iface = b.GetBluePrint(namespaceName)
							if iface == nil {
								iface = b.CreateBlueprint(namespaceName, ref)
							}
						} else {
							iface = b.GetBluePrint(namespaceName)
							if iface == nil {
								iface = app.GetClassBlueprintEx(namespaceName, strings.Join(pkgName, "."))
							}
							library, err := app.GetOrCreateLibrary(namespaceName)
							if err != nil {
								return
							}
							b.FakeGetBlueprint(library, namespaceName, ref)
						}
						iface.SetKind(ssa.BlueprintInterface)
						//todo： 待优化，优化到blueprint中
						blueprint.AddInterfaceBlueprint(iface)
					}
				})
			}
		}
	} else if i.Interface() != nil {
		name := y.VisitIdentifier(i.Identifier())
		blueprint = y.GetBluePrint(name)
		if blueprint == nil {
			blueprint = y.CreateBlueprint(name, i.Identifier())
		}
		blueprint.SetKind(ssa.BlueprintInterface)
		y.GetProgram().SetExportType(name, blueprint)
		if i.Extends() != nil {
			if i.InterfaceList() != nil {
				for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
					handlefunc = append(handlefunc, func(b *builder) {
						ref := impl.(*phpparser.QualifiedStaticTypeRefContext)
						pkgName, namespaceName := y.VisitQualifiedNamespaceName(ref.QualifiedNamespaceName())
						app := b.GetProgram().GetApplication()
						var iface *ssa.Blueprint
						if len(pkgName) <= 1 {
							iface = b.GetBluePrint(namespaceName)
							if iface == nil {
								iface = b.CreateInterface(namespaceName, ref.QualifiedNamespaceName())
							}
						} else {
							iface = b.GetBluePrint(namespaceName)
							if iface == nil {
								iface = app.GetClassBlueprintEx(namespaceName, strings.Join(pkgName, "."))
							}

							library, err := app.GetOrCreateLibrary(namespaceName)
							if err != nil {
								return
							}
							b.FakeGetBlueprint(library, namespaceName, ref.QualifiedNamespaceName())
						}
						iface.SetKind(ssa.BlueprintInterface)
						blueprint.AddParentBlueprint(iface)
					})
				}
			}
		}
	}
	if blueprint == nil {
		return
	}
	store := y.StoreFunctionBuilder()
	blueprint.AddLazyBuilder(func() {
		switchHandler := y.SwitchFunctionBuilder(store)
		defer switchHandler()
		for _, f := range handlefunc {
			f(y)
		}
	})
	for _, statement := range i.AllClassStatement() {
		y.VisitClassStatement(statement, blueprint)
	}
}

func (y *builder) VisitClassStatement(raw phpparser.IClassStatementContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	switch ret := raw.(type) {
	case *phpparser.PropertyModifiersVariableContext:
		modifiers := y.VisitPropertyModifiers(ret.PropertyModifiers())
		for _, va := range ret.AllVariableInitializer() {
			name, value := y.VisitVariableInitializer(va)
			if strings.HasPrefix(name, "$") {
				name = name[1:]
			}
			_, isStatic := modifiers[ssa.Static]
			if utils.IsNil(value) {
				value = y.EmitUndefined(name)
			}
			if isStatic {
				class.RegisterStaticMember(name, value)
				variable := y.GetStaticMember(class, name)
				y.AssignVariable(variable, value)
			} else {
				class.RegisterNormalMember(name, value)
			}
			currentBuilder := y.FunctionBuilder
			store := y.StoreFunctionBuilder()
			class.AddLazyBuilder(func() {
				switchHandler := y.SwitchFunctionBuilder(store)
				defer switchHandler()
				y.FunctionBuilder = currentBuilder
				typ := y.VisitTypeHint(ret.TypeHint())
				value.SetType(typ)
			})
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

		methodName := y.VisitIdentifier(ret.Identifier())
		funcName := fmt.Sprintf("%s_%s", class.Name, methodName)
		newFunction := y.NewFunc(funcName)
		newFunction.SetMethodName(methodName)
		store := y.StoreFunctionBuilder()
		newFunction.AddLazyBuilder(func() {
			switchHandler := y.SwitchFunctionBuilder(store)
			defer switchHandler()
			y.FunctionBuilder = y.PushFunction(newFunction)
			{
				var param ssa.Value
				if methodName == "__construct" {
					y.NewParam("0this")
					param = y.EmitEmptyContainer()
					y.AssignVariable(y.CreateVariable("$this"), param)
					param.SetType(class)
				} else if !isStatic {
					param = y.NewParam("$this")
					param.SetType(class)
				}
				y.VisitFormalParameterList(ret.FormalParameterList())
				y.VisitMethodBody(ret.MethodBody())
				if methodName == "__construct" {
					y.EmitReturn([]ssa.Value{param})
				}
			}
			y.Finish()
			y.FunctionBuilder = y.PopFunction()
		})

		switch methodName {
		case "__construct":
			newFunction.SetType(ssa.NewFunctionType(fmt.Sprintf("%s-__construct", class.Name), []ssa.Type{class}, class, true))
			class.RegisterMagicMethod(ssa.Constructor, newFunction)
		case "__destruct":
			class.RegisterMagicMethod(ssa.Destructor, newFunction)
		default:
			if isStatic {
				member := y.GetStaticMember(class, newFunction.GetName())
				_ = member.Assign(newFunction)
				class.RegisterStaticMethod(methodName, newFunction)
				//variable := y.GetStaticMember(class.Name, newFunction.GetName())
				//y.AssignVariable(variable, newFunction)
				//class.RegisterStaticMethod(funcName, newFunction)
			} else {
				class.RegisterNormalMethod(methodName, newFunction)
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
			class.RegisterConstMember(name, value)
		}

	case *phpparser.TraitUseContext:
	default:

	}
	return
}

func (y *builder) VisitTraitAdaptations(raw phpparser.ITraitAdaptationsContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
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
	if y == nil || raw == nil || y.IsStop() {
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
	if y == nil || raw == nil || y.IsStop() {
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
	if y == nil || raw == nil || y.IsStop() {
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
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.MethodBodyContext)
	if i.BlockStatement() != nil {

		y.VisitBlockStatement(i.BlockStatement())
	}

	return nil
}

func (y *builder) VisitIdentifierInitializer(raw phpparser.IIdentifierInitializerContext) (string, ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
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
	if y == nil || raw == nil || y.IsStop() {
		return "", nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
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
	if y == nil || raw == nil || y.IsStop() {
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

func (y *builder) VisitStaticClass(raw phpparser.IStaticClassContext) *ssa.Blueprint {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.StaticClassContext)
	if i == nil {
		return nil
	}

	var value ssa.Value
	var className string
	if full := i.FullyQualifiedNamespaceExpr(); full != nil {
		value = y.VisitFullyQualifiedNamespaceExpr(full, true)
	} else if variable := i.FlexiVariable(); variable != nil {
		value = y.VisitRightValue(variable)
	} else if i.String_() != nil {
		className = i.String_().GetText()
		if str, err := strconv.Unquote(className); err == nil {
			className = str
		}
	} else if i.Identifier() != nil {
		className = i.Identifier().GetText()
		if className == "parent" {
			parentClass := y.MarkedThisClassBlueprint.GetSuperBlueprint()
			if parentClass != nil {
				return parentClass
			}
		} else {
			blueprint := y.GetBluePrint(className)
			if blueprint != nil {
				return blueprint
			}
			blueprint = y.CreateBlueprint(className)
			return blueprint
		}
	} else {
		return nil
	}

	if value != nil {
		if bp, ok := ssa.ToClassBluePrintType(value.GetType()); ok {
			return bp
		}
		if bp := y.GetBluePrint(value.String()); bp != nil {
			return bp
		}
	}
	if className != "" {
		return y.GetBluePrint(className)
	}
	return nil
}

func (y *builder) VisitStaticClassExprFunctionMember(raw phpparser.IStaticClassExprFunctionMemberContext) (*ssa.Blueprint, string) {
	if y == nil || raw == nil {
		return nil, ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.StaticClassExprFunctionMemberContext)
	if i == nil {
		return nil, "'"
	}

	key := i.Identifier().GetText()
	if i.StaticClass().GetText() == "self" {
		return y.MarkedThisClassBlueprint, key
	}
	bluePrint := y.VisitStaticClass(i.StaticClass())
	return bluePrint, key
}

func (y *builder) VisitStaticClassExprVariableMember(raw phpparser.IStaticClassExprVariableMemberContext) (*ssa.Blueprint, string) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.StaticClassExprVariableMemberContext)
	if i == nil {
		return nil, "'"
	}
	key := yakunquote.TryUnquote(i.Variable().GetText())
	bluePrint := y.VisitStaticClass(i.StaticClass())
	if strings.HasPrefix(key, "$") {
		key = key[1:]
	}
	return bluePrint, key
}

func (y *builder) VisitStaticClassExpr(raw phpparser.IStaticClassExprContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	if i, ok := raw.(*phpparser.StaticClassExprContext); ok {
		if i.StaticClassExprFunctionMember() != nil {
			if bluePrint, key := y.VisitStaticClassExprFunctionMember(i.StaticClassExprFunctionMember()); bluePrint != nil {
				member := y.GetStaticMember(bluePrint, key)
				if value := y.PeekValueByVariable(member); !utils.IsNil(value) {
					return value
				}
				if method := bluePrint.GetStaticMethod(key); !utils.IsNil(method) {
					return method
				} else if member := bluePrint.GetConstMember(key); !utils.IsNil(member) {
					return member
				}
				undefined := y.EmitUndefined(key)
				bluePrint.RegisterStaticMember(key, undefined)
				return undefined
			}
		}
		if i.StaticClassExprVariableMember() != nil {
			if bluePrint, key := y.VisitStaticClassExprVariableMember(i.StaticClassExprVariableMember()); bluePrint != nil {
				variable := y.GetStaticMember(bluePrint, key)
				if val := y.PeekValueByVariable(variable); !utils.IsNil(val) {
					return val
				}
				if member := bluePrint.GetStaticMember(key); !utils.IsNil(member) {
					return member
				}
				return y.ReadMemberCallValue(bluePrint.Container(), y.EmitConstInst(key))
			}
		}
		return y.EmitUndefined(raw.GetText())
	}

	return y.EmitUndefined(raw.GetText())
}

/// class modifier

func (y *builder) VisitPropertyModifiers(raw phpparser.IPropertyModifiersContext) map[ssa.BlueprintModifier]struct{} {
	ret := make(map[ssa.BlueprintModifier]struct{})
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

func (y *builder) VisitMemberModifiers(raw phpparser.IMemberModifiersContext) map[ssa.BlueprintModifier]struct{} {
	ret := make(map[ssa.BlueprintModifier]struct{})
	i, ok := raw.(*phpparser.MemberModifiersContext)
	if !ok {
		return ret
	}

	for _, item := range i.AllMemberModifier() {
		ret[y.VisitMemberModifier(item)] = struct{}{}
	}

	return ret
}

func (y *builder) VisitMemberModifier(raw phpparser.IMemberModifierContext) ssa.BlueprintModifier {
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
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	if !ok {
		return nil
	}

	_ = i
	if i.Identifier() != nil {
		return y.EmitConstInstPlaceholder(y.VisitIdentifier(i.Identifier()))
	}

	if i.Variable() != nil {
		name := y.VisitVariable(i.Variable())
		value := y.ReadValue(name)
		if value.IsUndefined() {
			return y.EmitConstInstPlaceholder(strings.TrimPrefix(value.GetName(), "$"))
		} else {
			return y.EmitConstInstPlaceholder(value.String())
		}
	}

	if i.String_() != nil {
		return y.VisitString_(i.String_())
	}

	if ret := i.Expression(); ret != nil {
		return y.VisitExpression(ret)
	}

	return y.EmitUndefined(raw.GetText())
}

func (y *builder) VisitFullyQualifiedNamespaceExpr(raw phpparser.IFullyQualifiedNamespaceExprContext, wantBlueprint bool) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FullyQualifiedNamespaceExprContext)
	if i == nil {
		return nil
	}
	if !wantBlueprint {
		if _func, ok := y.GetFunc(i.GetText(), ""); ok {
			return _func
		}
	} else {
		bluePrint := y.GetBluePrint(i.GetText())
		if bluePrint != nil {
			inst := y.EmitConstInstPlaceholder(bluePrint.Name)
			inst.SetType(bluePrint)
			return inst
		}
	}
	var pkgPath []string
	for j := 0; j < len(i.AllIdentifier())-1; j++ {
		pkgPath = append(pkgPath, y.VisitIdentifier(i.Identifier(j)))
	}
	//获取最后一个identifier
	identifier := y.VisitIdentifier(i.Identifier(len(i.AllIdentifier()) - 1))
	program := y.GetProgram()
	library, err := program.GetOrCreateLibrary(strings.Join(pkgPath, "."))
	if err != nil {
		log.Errorf("create library fail: %s", err)
	} else if library != nil {
		if !wantBlueprint {
			value := library.GetExportValue(identifier)
			if !utils.IsNil(value) {
				return value
			}
		} else {
			fullTypeBluePrint := library.GetBluePrint(identifier, raw)
			if !utils.IsNil(fullTypeBluePrint) {
				undefined := y.EmitUndefined(identifier)
				undefined.SetType(fullTypeBluePrint)
				return undefined
			}
		}
	}
	undefined := y.EmitUndefined(identifier)
	if wantBlueprint {
		bluePrint := y.CreateBlueprint(identifier)
		bluePrint.SetFullTypeNames(pkgPath)
		undefined.SetType(bluePrint)
		return undefined
	} else {
		return undefined
	}
}

func (y *builder) ResolveValue(name string) ssa.Value {
	if value := y.PeekValue(name); value != nil {
		// found
		return value
	}
	if className := y.MarkedThisClassBlueprint; className != nil {
		if value, ok := y.ReadClassConst(className.Name, name); ok {
			return value
		}
		value := y.ReadSelfMember(name)
		if value != nil {
			return value
		}
	}
	if value, ok := y.ReadConst(name); ok {
		return value
	}
	return y.ReadValue(name)
}
