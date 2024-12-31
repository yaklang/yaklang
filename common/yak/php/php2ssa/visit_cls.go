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
	cname := fmt.Sprintf("anonymous_%s_%s", y.CurrentFile, y.CurrentRange.GetStart())
	bluePrint := y.CreateBluePrint(cname)
	if i.QualifiedStaticTypeRef() != nil {
		if ref := y.VisitQualifiedStaticTypeRef(i.QualifiedStaticTypeRef()); ref != nil {
			bluePrint.AddParentClass(ref)
		}
	}
	for _, statement := range i.AllClassStatement() {
		y.VisitClassStatement(statement, bluePrint)
	}
	//todo: 可能会有问题
	// bluePrint.Build()
	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(bluePrint)
	args := []ssa.Value{obj}
	ellipsis := false
	if i.Arguments() != nil {
		tmp, hasEllipsis := y.VisitArguments(i.Arguments())
		ellipsis = hasEllipsis
		args = append(args, tmp...)
	}
	_ = ellipsis
	return y.ClassConstructor(bluePrint, args)
}

func (y *builder) VisitClassDeclaration(raw phpparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
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

			class := y.CreateBluePrint(className)
			y.MarkedThisClassBlueprint = class
			y.GetProgram().SetExportType(className, class)
			if i.Extends() != nil {
				parentClassName = i.QualifiedStaticTypeRef().GetText()
				store := y.StoreFunctionBuilder()
				class.AddLazyBuilder(func() {
					switchHandler := y.SwitchFunctionBuilder(store)
					defer switchHandler()
					if parentClass := y.GetBluePrint(parentClassName); parentClass != nil {
						//感觉在ssa-classBlue中做更好，暂时修复
						class.AddParentClass(parentClass)
						class.AddSuperBlueprint(parentClass)
						for _, s := range parentClass.GetFullTypeNames() {
							class.AddFullTypeName(s)
						}
					}
				})
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
				} else {
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
		value = y.VisitFullyQualifiedNamespaceExpr(full)
	} else if variable := i.Variable(); variable != nil {
		name := y.VisitVariable(variable)
		value = y.ReadValue(name)
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

	value := y.VisitRightValue(i.FlexiVariable())
	key := value.GetName()
	if key == "" {
		key = value.String()
	}
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
				return y.EmitUndefined(raw.GetText())
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
			}
		}
		return y.EmitUndefined(raw.GetText())
	}

	return y.EmitUndefined(raw.GetText())
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
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	if !ok {
		return nil
	}

	_ = i
	if i.Identifier() != nil {
		return y.EmitConstInst(y.VisitIdentifier(i.Identifier()))
	}

	if i.Variable() != nil {
		name := y.VisitVariable(i.Variable())
		value := y.ReadValue(name)
		if value.IsUndefined() {
			return y.EmitConstInst(strings.TrimPrefix(value.GetName(), "$"))
		} else {
			return y.EmitConstInst(value.String())
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

func (y *builder) VisitFullyQualifiedNamespaceExpr(raw phpparser.IFullyQualifiedNamespaceExprContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.FullyQualifiedNamespaceExprContext)
	if i == nil {
		return nil
	}
	var pkgPath []string
	for j := 0; j < len(i.AllIdentifier())-1; j++ {
		pkgPath = append(pkgPath, y.VisitIdentifier(i.Identifier(j)))
	}
	//获取最后一个identifier
	identifier := y.VisitIdentifier(i.Identifier(len(i.AllIdentifier()) - 1))
	program := y.GetProgram()
	library, b := program.GetLibrary(strings.Join(pkgPath, "."))
	if b {
		if function, ok := library.Funcs.Get(identifier); ok && !utils.IsNil(function) {
			return function
		} else if cls := library.GetBluePrint(identifier); !utils.IsNil(cls) {
			inst := y.EmitConstInst("")
			inst.SetType(cls)
			return inst
		} else {
			return y.EmitUndefined(raw.GetText())
		}
	}
	return y.EmitUndefined(raw.GetText())
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
