package csharp2ssa

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// nestedTypeSplit 嵌套类型蓝图名分隔符：Outer$Inner
const nestedTypeSplit = "$"

// ensureBlueprintMemberSlot pre-declares a class member so later storeField
// (RegisterStaticMethod / RegisterNormalMethod / constructor magic method)
// does not emit ObjectError for a missing member.
func (b *singleFileBuilder) ensureBlueprintMemberSlot(blueprint *ssa.Blueprint, name string, static bool) {
	if b == nil || blueprint == nil || name == "" {
		return
	}
	if static {
		if utils.IsNil(blueprint.GetStaticMember(name)) {
			blueprint.RegisterStaticMember(name, b.EmitUndefined(name), false)
		}
		return
	}
	if utils.IsNil(blueprint.GetNormalMember(name)) {
		blueprint.RegisterNormalMember(name, b.EmitUndefined(name), false)
	}
}

func (b *singleFileBuilder) ensureBlueprintConstructorSlot(blueprint *ssa.Blueprint) {
	if blueprint == nil || blueprint.Name == "" {
		return
	}
	b.ensureBlueprintMemberSlot(blueprint, blueprint.Name, false)
	b.ensureBlueprintMemberSlot(blueprint, blueprint.Name, true)
}

// declareBlueprint 创建（或复用 partial）源码声明的蓝图，并登记导出与全名。
func (b *singleFileBuilder) declareBlueprint(name string, kind ssa.BlueprintKind, token ssa.CanStartStopToken, outer *ssa.Blueprint) *ssa.Blueprint {
	if name == "" {
		return nil
	}
	fullName := name
	if outer != nil {
		fullName = outer.Name + nestedTypeSplit + name
	}
	var bp *ssa.Blueprint
	if token != nil {
		bp = b.CreateBlueprint(fullName, token)
	} else {
		bp = b.CreateBlueprint(fullName)
	}
	if bp == nil {
		return nil
	}
	b.constructors.markDeclared(bp)
	bp.SetKind(kind)
	if prog := b.GetProgram(); prog != nil {
		prog.SetExportType(fullName, bp)
	}
	b.declaredFullTypeName(bp)
	b.ensureBlueprintConstructorSlot(bp)
	if outer != nil {
		b.ensureBlueprintMemberSlot(outer, name, true)
		outer.RegisterStaticMember(name, bp.Container())
	}
	return bp
}

// withClassContext 在访问类成员期间设置 MarkedThisClassBlueprint / BlueprintStack。
func (b *singleFileBuilder) withClassContext(bp *ssa.Blueprint, fn func()) {
	prev := b.MarkedThisClassBlueprint
	b.MarkedThisClassBlueprint = bp
	b.PushBlueprint(bp)
	defer func() {
		b.PopBlueprint()
		b.MarkedThisClassBlueprint = prev
	}()
	fn()
}

// ---------------------------------------------------------------- class

func (b *singleFileBuilder) VisitClassDeclaration(raw csharpparser.IClass_declarationContext, outer *ssa.Blueprint) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Class_declarationContext)
	if !ok || i == nil {
		return nil
	}
	bp := b.declareBlueprint(identText(i.Identifier()), ssa.BlueprintClass, i.Identifier(), outer)
	if bp == nil {
		return nil
	}
	if cb, _ := i.Class_base().(*csharpparser.Class_baseContext); cb != nil {
		b.registerBaseTypes(bp, cb.Class_type(), cb.Interface_type_list())
	}
	b.withClassContext(bp, func() {
		if body, _ := i.Class_body().(*csharpparser.Class_bodyContext); body != nil {
			for _, member := range body.AllClass_member_declaration() {
				b.VisitClassMemberDeclaration(member, bp)
			}
		}
	})
	return bp.Container()
}

// registerBaseTypes 延迟解析父类 / 接口（此时所有骨架已建立）。
func (b *singleFileBuilder) registerBaseTypes(bp *ssa.Blueprint, classType csharpparser.IClass_typeContext, ifaces csharpparser.IInterface_type_listContext) {
	if bp == nil || (classType == nil && ifaces == nil) {
		return
	}
	classType = ssa.DetachAST(classType)
	ifaces = ssa.DetachAST(ifaces)
	b.lazyBuild(bp.AddLazyBuilder, func() {
		hasParent := false
		if classType != nil {
			if parent, ok := ssa.ToBluePrintType(b.visitClassType(classType)); ok && parent != nil && parent != bp {
				// The parser defaults an unresolved single base-list entry to
				// class_type. Preserve a known interface (or the conventional IName
				// shape) instead of turning `class C : IDisposable` into class
				// inheritance merely because dependency metadata is unavailable.
				if parent.IsInterface() || looksLikeInterfaceName(parent.Name) {
					parent.SetKind(ssa.BlueprintInterface)
					bp.AddInterfaceBlueprintRelationOnly(parent)
				} else {
					parent.SetKind(ssa.BlueprintClass)
					bp.AddParentBlueprintRelationOnly(parent)
					hasParent = true
				}
			}
		}
		list, _ := ifaces.(*csharpparser.Interface_type_listContext)
		if list == nil {
			return
		}
		for idx, it := range list.AllInterface_type() {
			parent, ok := ssa.ToBluePrintType(b.visitInterfaceType(it))
			if !ok || parent == nil || parent == bp {
				continue
			}
			switch {
			case parent.IsInterface():
				bp.AddInterfaceBlueprintRelationOnly(parent)
			case b.isDeclaredBlueprint(parent.Name):
				// 源码声明且不是接口：只能是基类
				bp.AddParentBlueprintRelationOnly(parent)
				hasParent = true
			case idx == 0 && !hasParent && !bp.IsInterface() && !looksLikeInterfaceName(parent.Name):
				parent.SetKind(ssa.BlueprintClass)
				bp.AddParentBlueprintRelationOnly(parent)
				hasParent = true
			default:
				parent.SetKind(ssa.BlueprintInterface)
				bp.AddInterfaceBlueprintRelationOnly(parent)
			}
		}
	})
}

// looksLikeInterfaceName 遵循 .NET 命名约定：IDisposable / IEnumerable<T>
func looksLikeInterfaceName(name string) bool {
	if len(name) < 2 || name[0] != 'I' {
		return false
	}
	c := name[1]
	return c >= 'A' && c <= 'Z'
}

func (b *singleFileBuilder) VisitStructDeclaration(raw csharpparser.IStruct_declarationContext, outer *ssa.Blueprint) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Struct_declarationContext)
	if !ok || i == nil {
		return nil
	}
	bp := b.declareBlueprint(identText(i.Identifier()), ssa.BlueprintClass, i.Identifier(), outer)
	if bp == nil {
		return nil
	}
	if si, _ := i.Struct_interfaces().(*csharpparser.Struct_interfacesContext); si != nil {
		b.registerBaseTypes(bp, nil, si.Interface_type_list())
	}
	b.withClassContext(bp, func() {
		if body, _ := i.Struct_body().(*csharpparser.Struct_bodyContext); body != nil {
			for _, member := range body.AllStruct_member_declaration() {
				b.VisitStructMemberDeclaration(member, bp)
			}
		}
	})
	return bp.Container()
}

func (b *singleFileBuilder) VisitInterfaceDeclaration(raw csharpparser.IInterface_declarationContext, outer *ssa.Blueprint) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Interface_declarationContext)
	if !ok || i == nil {
		return nil
	}
	bp := b.declareBlueprint(identText(i.Identifier()), ssa.BlueprintInterface, i.Identifier(), outer)
	if bp == nil {
		return nil
	}
	if ib, _ := i.Interface_base().(*csharpparser.Interface_baseContext); ib != nil {
		b.registerBaseTypes(bp, nil, ib.Interface_type_list())
	}
	b.withClassContext(bp, func() {
		if body, _ := i.Interface_body().(*csharpparser.Interface_bodyContext); body != nil {
			for _, member := range body.AllInterface_member_declaration() {
				b.VisitInterfaceMemberDeclaration(member, bp)
			}
		}
	})
	return bp.Container()
}

func (b *singleFileBuilder) VisitEnumDeclaration(raw csharpparser.IEnum_declarationContext, outer *ssa.Blueprint) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Enum_declarationContext)
	if !ok || i == nil {
		return
	}
	bp := b.declareBlueprint(identText(i.Identifier()), ssa.BlueprintClass, i.Identifier(), outer)
	if bp == nil {
		return
	}
	body, _ := i.Enum_body().(*csharpparser.Enum_bodyContext)
	if body == nil {
		return
	}
	decls, _ := body.Enum_member_declarations().(*csharpparser.Enum_member_declarationsContext)
	if decls == nil {
		return
	}
	type enumMember struct {
		name string
		expr csharpparser.IConstant_expressionContext
	}
	members := make([]enumMember, 0)
	for _, m := range decls.AllEnum_member_declaration() {
		mc, _ := m.(*csharpparser.Enum_member_declarationContext)
		if mc == nil {
			continue
		}
		name := identText(mc.Identifier())
		if name == "" {
			continue
		}
		b.ensureBlueprintMemberSlot(bp, name, true)
		members = append(members, enumMember{name: name, expr: ssa.DetachAST(mc.Constant_expression())})
	}
	b.lazyBuild(bp.AddLazyBuilder, func() {
		b.withClassContext(bp, func() {
			var next int64
			for _, m := range members {
				var value ssa.Value
				if ce, _ := m.expr.(*csharpparser.Constant_expressionContext); ce != nil {
					value = b.VisitExpression(ce.Expression())
					if c, ok := ssa.ToConstInst(value); ok && c.IsNumber() {
						next = c.Number() + 1
					}
				}
				if utils.IsNil(value) {
					value = b.EmitConstInst(next)
					next++
				}
				value.SetType(bp)
				bp.RegisterStaticMember(m.name, value)
				bp.RegisterConstMember(m.name, value, false)
			}
		})
	})
}

func (b *singleFileBuilder) VisitDelegateDeclaration(raw csharpparser.IDelegate_declarationContext, outer *ssa.Blueprint) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Delegate_declarationContext)
	if !ok || i == nil {
		return
	}
	header, _ := i.Delegate_header().(*csharpparser.Delegate_headerContext)
	if header == nil {
		return
	}
	bp := b.declareBlueprint(identText(header.Identifier()), ssa.BlueprintClass, header.Identifier(), outer)
	if bp == nil {
		return
	}
	params := ssa.DetachAST(header.Parameter_list())
	invoke := b.declareMethod(bp, "Invoke", false)
	b.buildFunctionLazy(invoke, bp, false, func() {
		b.VisitParameterList(params)
	}, nil)
}

// ---------------------------------------------------------------- members

func (b *singleFileBuilder) VisitClassMemberDeclaration(raw csharpparser.IClass_member_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Class_member_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Constant_declaration() != nil:
		b.VisitConstantDeclaration(i.Constant_declaration(), class)
	case i.Field_declaration() != nil:
		b.VisitFieldDeclaration(i.Field_declaration(), class)
	case i.Method_declaration() != nil:
		b.VisitMethodDeclaration(i.Method_declaration(), class)
	case i.Property_declaration() != nil:
		b.VisitPropertyDeclaration(i.Property_declaration(), class)
	case i.Event_declaration() != nil:
		b.VisitEventDeclaration(i.Event_declaration(), class)
	case i.Indexer_declaration() != nil:
		b.VisitIndexerDeclaration(i.Indexer_declaration(), class)
	case i.Operator_declaration() != nil:
		b.VisitOperatorDeclaration(i.Operator_declaration(), class)
	case i.Constructor_declaration() != nil:
		b.VisitConstructorDeclaration(i.Constructor_declaration(), class)
	case i.Finalizer_declaration() != nil:
		b.VisitFinalizerDeclaration(i.Finalizer_declaration(), class)
	case i.Static_constructor_declaration() != nil:
		b.VisitStaticConstructorDeclaration(i.Static_constructor_declaration(), class)
	case i.Type_declaration() != nil:
		b.VisitTypeDeclaration(i.Type_declaration(), class)
	}
}

func (b *singleFileBuilder) VisitStructMemberDeclaration(raw csharpparser.IStruct_member_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Struct_member_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Constant_declaration() != nil:
		b.VisitConstantDeclaration(i.Constant_declaration(), class)
	case i.Field_declaration() != nil:
		b.VisitFieldDeclaration(i.Field_declaration(), class)
	case i.Method_declaration() != nil:
		b.VisitMethodDeclaration(i.Method_declaration(), class)
	case i.Property_declaration() != nil:
		b.VisitPropertyDeclaration(i.Property_declaration(), class)
	case i.Event_declaration() != nil:
		b.VisitEventDeclaration(i.Event_declaration(), class)
	case i.Indexer_declaration() != nil:
		b.VisitIndexerDeclaration(i.Indexer_declaration(), class)
	case i.Operator_declaration() != nil:
		b.VisitOperatorDeclaration(i.Operator_declaration(), class)
	case i.Constructor_declaration() != nil:
		b.VisitConstructorDeclaration(i.Constructor_declaration(), class)
	case i.Static_constructor_declaration() != nil:
		b.VisitStaticConstructorDeclaration(i.Static_constructor_declaration(), class)
	case i.Type_declaration() != nil:
		b.VisitTypeDeclaration(i.Type_declaration(), class)
	}
}

func (b *singleFileBuilder) VisitInterfaceMemberDeclaration(raw csharpparser.IInterface_member_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Interface_member_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Constant_declaration() != nil:
		b.VisitConstantDeclaration(i.Constant_declaration(), class)
	case i.Field_declaration() != nil:
		b.VisitFieldDeclaration(i.Field_declaration(), class)
	case i.Method_declaration() != nil:
		b.VisitMethodDeclaration(i.Method_declaration(), class)
	case i.Property_declaration() != nil:
		b.VisitPropertyDeclaration(i.Property_declaration(), class)
	case i.Event_declaration() != nil:
		b.VisitEventDeclaration(i.Event_declaration(), class)
	case i.Indexer_declaration() != nil:
		b.VisitIndexerDeclaration(i.Indexer_declaration(), class)
	case i.Static_constructor_declaration() != nil:
		b.VisitStaticConstructorDeclaration(i.Static_constructor_declaration(), class)
	case i.Operator_declaration() != nil:
		b.VisitOperatorDeclaration(i.Operator_declaration(), class)
	case i.Type_declaration() != nil:
		b.VisitTypeDeclaration(i.Type_declaration(), class)
	}
}

// ---------------------------------------------------------------- const / field

func (b *singleFileBuilder) VisitConstantDeclaration(raw csharpparser.IConstant_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Constant_declarationContext)
	if !ok || i == nil {
		return
	}
	dc, _ := i.Constant_declarators().(*csharpparser.Constant_declaratorsContext)
	if dc == nil {
		return
	}
	typeCtx := ssa.DetachAST(i.Type_())
	for _, d := range dc.AllConstant_declarator() {
		cd, _ := d.(*csharpparser.Constant_declaratorContext)
		if cd == nil {
			continue
		}
		name := identText(cd.Identifier())
		if name == "" {
			continue
		}
		b.ensureBlueprintMemberSlot(class, name, true)
		expr := ssa.DetachAST(cd.Constant_expression())
		b.lazyBuild(class.AddLazyBuilder, func() {
			b.withClassContext(class, func() {
				var value ssa.Value
				if ce, _ := expr.(*csharpparser.Constant_expressionContext); ce != nil {
					value = b.VisitExpression(ce.Expression())
				}
				if utils.IsNil(value) {
					return
				}
				value = b.applyDeclaredType(value, b.VisitType(typeCtx))
				class.RegisterStaticMember(name, value)
				class.RegisterConstMember(name, value, false)
				b.constMap[name] = value
			})
		})
	}
}

func (b *singleFileBuilder) VisitFieldDeclaration(raw csharpparser.IField_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Field_declarationContext)
	if !ok || i == nil {
		return
	}
	isStatic := false
	for _, mod := range i.AllField_modifier() {
		if mc, _ := mod.(*csharpparser.Field_modifierContext); mc != nil && mc.KW_STATIC() != nil {
			isStatic = true
		}
	}
	dc, _ := i.Variable_declarators().(*csharpparser.Variable_declaratorsContext)
	if dc == nil {
		return
	}
	typeCtx := ssa.DetachAST(i.Type_())
	for _, d := range dc.AllVariable_declarator() {
		vd, _ := d.(*csharpparser.Variable_declaratorContext)
		if vd == nil {
			continue
		}
		name := identText(vd.Identifier())
		if name == "" {
			continue
		}
		b.declareFieldLike(class, name, isStatic, typeCtx, ssa.DetachAST(vd.Variable_initializer()), vd.Identifier())
	}
}

// declareFieldLike 登记字段 / 自动属性 / 事件这类「成员槽位」，并延迟计算初始值和类型。
func (b *singleFileBuilder) declareFieldLike(class *ssa.Blueprint, name string, isStatic bool, typeCtx csharpparser.IType_Context, init csharpparser.IVariable_initializerContext, token ssa.CanStartStopToken) {
	if class == nil || name == "" {
		return
	}
	setMember := class.RegisterNormalMember
	if isStatic {
		setMember = class.RegisterStaticMember
	}
	var placeholder ssa.Value
	if token != nil {
		recover := b.SetRange(token)
		placeholder = b.EmitUndefined(name)
		recover()
	} else {
		placeholder = b.EmitUndefined(name)
	}
	setMember(name, placeholder, false)
	b.lazyBuild(class.AddLazyBuilder, func() {
		b.withClassContext(class, func() {
			typ := b.VisitType(typeCtx)
			b.registerDeclaredMemberType(class, name, isStatic, typ)
			if typ != nil && typ.GetTypeKind() != ssa.AnyTypeKind {
				placeholder.SetType(typ)
			}
			if init == nil {
				return
			}
			value := b.VisitVariableInitializer(init)
			if utils.IsNil(value) {
				return
			}
			value = b.applyDeclaredType(value, typ)
			setMember(name, value)
		})
	})
}

func (b *singleFileBuilder) VisitVariableInitializer(raw csharpparser.IVariable_initializerContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Variable_initializerContext)
	if !ok || i == nil {
		return nil
	}
	if i.Expression() != nil {
		return b.VisitExpression(i.Expression())
	}
	if i.Array_initializer() != nil {
		return b.VisitArrayInitializer(i.Array_initializer(), nil)
	}
	return nil
}

// ---------------------------------------------------------------- method helpers

func (b *singleFileBuilder) newMethodFunction(class *ssa.Blueprint, name string, static bool) *ssa.Function {
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, name, uuid.NewString()[:4])
	f := b.NewFunc(funcName)
	f.SetMethodName(name)
	if !static {
		f.SetMethod(true, class)
	}
	return f
}

func (b *singleFileBuilder) registerBlueprintMethod(class *ssa.Blueprint, name string, static bool, f *ssa.Function) {
	b.ensureBlueprintMemberSlot(class, name, static)
	if static {
		class.RegisterStaticMethod(name, f)
	} else {
		class.RegisterNormalMethod(name, f)
	}
}

func (b *singleFileBuilder) registerBlueprintSourceMethod(class *ssa.Blueprint, name string, static bool, f *ssa.Function) {
	b.ensureBlueprintMemberSlot(class, name, static)
	if static {
		class.RegisterStaticMethodExact(name, f)
	} else {
		class.RegisterNormalMethodExact(name, f)
	}
}

// declareMethod creates the function skeleton and registers it on the blueprint.
// It remains the entry point for accessors/operators, which do not participate
// in source-method overload binding.
func (b *singleFileBuilder) declareMethod(class *ssa.Blueprint, name string, static bool) *ssa.Function {
	f := b.newMethodFunction(class, name, static)
	b.registerBlueprintMethod(class, name, static, f)
	return f
}

// buildFunctionLazy 注册方法体的延迟构建：实例方法注入 this 参数。
func (b *singleFileBuilder) buildFunctionLazy(f *ssa.Function, class *ssa.Blueprint, static bool, params func(), body func()) {
	if f == nil {
		return
	}
	b.lazyBuild(f.AddLazyBuilder, func() {
		b.FunctionBuilder = b.PushFunction(f)
		if !static && class != nil {
			this := b.NewParam("this")
			this.SetType(class)
		}
		b.MarkedThisClassBlueprint = class
		if params != nil {
			params()
		}
		if body != nil {
			body()
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}

func methodIsStatic(raw csharpparser.IMethod_modifiersContext) bool {
	i, _ := raw.(*csharpparser.Method_modifiersContext)
	if i == nil {
		return false
	}
	for _, m := range i.AllMethod_modifier() {
		mc, _ := m.(*csharpparser.Method_modifierContext)
		if mc == nil {
			continue
		}
		if rm, _ := mc.Ref_method_modifier().(*csharpparser.Ref_method_modifierContext); rm != nil && rm.KW_STATIC() != nil {
			return true
		}
	}
	return false
}

func refMethodIsStatic(raw csharpparser.IRef_method_modifiersContext) bool {
	i, _ := raw.(*csharpparser.Ref_method_modifiersContext)
	if i == nil {
		return false
	}
	for _, m := range i.AllRef_method_modifier() {
		if rm, _ := m.(*csharpparser.Ref_method_modifierContext); rm != nil && rm.KW_STATIC() != nil {
			return true
		}
	}
	return false
}

func methodDispatchFlags(methods csharpparser.IMethod_modifiersContext, refs csharpparser.IRef_method_modifiersContext) (override, hides bool) {
	visit := func(raw csharpparser.IRef_method_modifierContext) {
		modifier, _ := raw.(*csharpparser.Ref_method_modifierContext)
		if modifier == nil {
			return
		}
		override = override || modifier.KW_OVERRIDE() != nil
		hides = hides || modifier.KW_NEW() != nil
	}
	if list, _ := methods.(*csharpparser.Method_modifiersContext); list != nil {
		for _, raw := range list.AllMethod_modifier() {
			modifier, _ := raw.(*csharpparser.Method_modifierContext)
			if modifier != nil {
				visit(modifier.Ref_method_modifier())
			}
		}
	}
	if list, _ := refs.(*csharpparser.Ref_method_modifiersContext); list != nil {
		for _, raw := range list.AllRef_method_modifier() {
			visit(raw)
		}
	}
	return
}

func methodIsPartial(raw csharpparser.IMethod_modifiersContext) bool {
	modifiers, _ := raw.(*csharpparser.Method_modifiersContext)
	return modifiers != nil && modifiers.KW_PARTIAL() != nil
}

func methodHasImplementation(body csharpparser.IMethod_bodyContext, refBody csharpparser.IRef_method_bodyContext) bool {
	if body != nil && strings.TrimSpace(body.GetText()) != ";" {
		return true
	}
	return refBody != nil && strings.TrimSpace(refBody.GetText()) != ";"
}

func memberNameText(raw csharpparser.IMember_nameContext) string {
	i, _ := raw.(*csharpparser.Member_nameContext)
	if i == nil {
		return ""
	}
	return identText(i.Identifier())
}

func (b *singleFileBuilder) VisitMethodDeclaration(raw csharpparser.IMethod_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Method_declarationContext)
	if !ok || i == nil {
		return
	}
	header, _ := i.Method_header().(*csharpparser.Method_headerContext)
	if header == nil {
		return
	}
	key := memberNameText(header.Member_name())
	if key == "" {
		return
	}
	isStatic := methodIsStatic(i.Method_modifiers()) || refMethodIsStatic(i.Ref_method_modifiers())
	isOverride, hides := methodDispatchFlags(i.Method_modifiers(), i.Ref_method_modifiers())
	isPartial := methodIsPartial(i.Method_modifiers())
	hasBody := methodHasImplementation(i.Method_body(), i.Ref_method_body())
	if class.IsInterface() {
		isStatic = isStatic || false
	}
	f := b.newMethodFunction(class, key, isStatic)
	if b.constructors.registerMethod(class, key, isStatic, f, header.Parameter_list(), isOverride, hides, isPartial, hasBody) {
		b.registerBlueprintSourceMethod(class, key, isStatic, f)
	}
	params := ssa.DetachAST(header.Parameter_list())
	returnType := ssa.DetachAST(i.Return_type())
	refReturnType := ssa.DetachAST(i.Ref_return_type())
	body := ssa.DetachAST(i.Method_body())
	refBody := ssa.DetachAST(i.Ref_method_body())
	b.buildFunctionLazy(f, class, isStatic, func() {
		if returnType != nil {
			b.SetCurrentReturnType(b.VisitReturnType(returnType))
		} else if rt, _ := refReturnType.(*csharpparser.Ref_return_typeContext); rt != nil {
			b.SetCurrentReturnType(b.VisitType(rt.Type_()))
		}
		b.VisitParameterList(params)
	}, func() {
		if body != nil {
			b.VisitMethodBody(body)
			return
		}
		if rb, _ := refBody.(*csharpparser.Ref_method_bodyContext); rb != nil {
			if rb.Block() != nil {
				b.visitFunctionBodyBlock(rb.Block())
			} else if rb.Variable_reference() != nil {
				if v := b.VisitVariableReference(rb.Variable_reference()); !utils.IsNil(v) {
					b.EmitReturn([]ssa.Value{v})
				}
			}
		}
	})
}

func (b *singleFileBuilder) VisitMethodBody(raw csharpparser.IMethod_bodyContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Method_bodyContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Block() != nil:
		b.visitFunctionBodyBlock(i.Block())
	case i.Expression() != nil:
		if v := b.VisitExpression(i.Expression()); !utils.IsNil(v) {
			b.EmitReturn([]ssa.Value{v})
		}
	case i.Null_conditional_invocation_expression() != nil:
		if v := b.VisitNullConditionalInvocationExpression(i.Null_conditional_invocation_expression()); !utils.IsNil(v) {
			b.EmitReturn([]ssa.Value{v})
		}
	}
}

func (b *singleFileBuilder) VisitVariableReference(raw csharpparser.IVariable_referenceContext) ssa.Value {
	i, _ := raw.(*csharpparser.Variable_referenceContext)
	if i == nil {
		return nil
	}
	return b.VisitExpression(i.Expression())
}

// VisitParameterList declares parameters for the current function.
func (b *singleFileBuilder) VisitParameterList(raw csharpparser.IParameter_listContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Parameter_listContext)
	if !ok || i == nil {
		return
	}
	if fp, _ := i.Fixed_parameters().(*csharpparser.Fixed_parametersContext); fp != nil {
		for _, p := range fp.AllFixed_parameter() {
			b.VisitFixedParameter(p)
		}
	}
	if pa, _ := i.Parameter_array().(*csharpparser.Parameter_arrayContext); pa != nil {
		name := identText(pa.Identifier())
		if name != "" {
			param := b.NewParam(name, pa.Identifier())
			if pa.Array_type() != nil {
				b.rememberDeclaredParameterType(param, b.visitArrayType(pa.Array_type()))
			}
			b.HandlerEllipsis()
		}
	}
}

func (b *singleFileBuilder) VisitFixedParameter(raw csharpparser.IFixed_parameterContext) *ssa.Parameter {
	pc, _ := raw.(*csharpparser.Fixed_parameterContext)
	if pc == nil {
		return nil
	}
	name := identText(pc.Identifier())
	if name == "" {
		return nil
	}
	param := b.NewParam(name, pc.Identifier())
	modifier := csharpFixedParameterModifier(pc)
	if modifier == "ref" || modifier == "out" {
		// FormalParameterIndex already includes the synthetic instance `this`,
		// which is exactly the Call.Args index consumed by PointerSideEffect.
		b.ReferenceParameter(name, param.FormalParameterIndex, ssa.PointerSideEffect)
	}
	if pc.Type_() != nil {
		b.rememberDeclaredParameterType(param, b.VisitType(pc.Type_()))
	}
	if da, _ := pc.Default_argument().(*csharpparser.Default_argumentContext); da != nil && da.Expression() != nil {
		if v := b.VisitExpression(da.Expression()); !utils.IsNil(v) {
			param.SetDefault(v)
		}
	}
	return param
}

// ---------------------------------------------------------------- property / event / indexer

func propertyIsStatic(mods []csharpparser.IProperty_modifierContext) bool {
	for _, m := range mods {
		if mc, _ := m.(*csharpparser.Property_modifierContext); mc != nil && mc.KW_STATIC() != nil {
			return true
		}
	}
	return false
}

func propertyDispatchFlags(mods []csharpparser.IProperty_modifierContext) (override, hides bool) {
	for _, raw := range mods {
		modifier, _ := raw.(*csharpparser.Property_modifierContext)
		if modifier == nil {
			continue
		}
		override = override || modifier.KW_OVERRIDE() != nil
		hides = hides || modifier.KW_NEW() != nil
	}
	return
}

func indexerDispatchFlags(mods []csharpparser.IIndexer_modifierContext) (override, hides bool) {
	for _, raw := range mods {
		modifier, _ := raw.(*csharpparser.Indexer_modifierContext)
		if modifier == nil {
			continue
		}
		override = override || modifier.KW_OVERRIDE() != nil
		hides = hides || modifier.KW_NEW() != nil
	}
	return
}

type csharpAccessorOptions struct {
	parameters []csharpConstructorParameter
	override   bool
	hides      bool
}

func (b *singleFileBuilder) VisitPropertyDeclaration(raw csharpparser.IProperty_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Property_declarationContext)
	if !ok || i == nil {
		return
	}
	name := memberNameText(i.Member_name())
	if name == "" {
		return
	}
	isStatic := propertyIsStatic(i.AllProperty_modifier())
	isOverride, hides := propertyDispatchFlags(i.AllProperty_modifier())
	typeCtx := ssa.DetachAST(i.Type_())
	typeName := ""
	if typeCtx != nil {
		typeName = typeCtx.GetText()
	}
	getterOptions := csharpAccessorOptions{override: isOverride, hides: hides}
	setterOptions := csharpAccessorOptions{
		parameters: []csharpConstructorParameter{{name: "value", typeName: typeName}},
		override:   isOverride,
		hides:      hides,
	}
	var token ssa.CanStartStopToken
	if mn, _ := i.Member_name().(*csharpparser.Member_nameContext); mn != nil && mn.Identifier() != nil {
		token = mn.Identifier()
	}

	if body, _ := i.Property_body().(*csharpparser.Property_bodyContext); body != nil {
		var init csharpparser.IVariable_initializerContext
		if pi, _ := body.Property_initializer().(*csharpparser.Property_initializerContext); pi != nil {
			init = pi.Variable_initializer()
		}
		b.declareFieldLike(class, name, isStatic, typeCtx, ssa.DetachAST(init), token)
		if body.Expression() != nil {
			b.declareAccessor(class, "get_"+name, isStatic, nil, ssa.DetachAST(body.Expression()), nil, nil, getterOptions)
			return
		}
		if acc, _ := body.Accessor_declarations().(*csharpparser.Accessor_declarationsContext); acc != nil {
			if g, _ := acc.Get_accessor_declaration().(*csharpparser.Get_accessor_declarationContext); g != nil {
				b.declareAccessorFromBody(class, "get_"+name, isStatic, g.Accessor_body(), nil, getterOptions)
			}
			if s, _ := acc.Set_accessor_declaration().(*csharpparser.Set_accessor_declarationContext); s != nil {
				b.declareAccessorFromBody(class, "set_"+name, isStatic, s.Accessor_body(), func() {
					p := b.NewParam("value")
					p.SetType(b.VisitType(typeCtx))
				}, setterOptions)
			}
		}
		return
	}
	if rb, _ := i.Ref_property_body().(*csharpparser.Ref_property_bodyContext); rb != nil {
		b.declareFieldLike(class, name, isStatic, typeCtx, nil, token)
		if rb.Variable_reference() != nil {
			b.declareAccessor(class, "get_"+name, isStatic, nil, nil, ssa.DetachAST(rb.Variable_reference()), nil, getterOptions)
		} else if g, _ := rb.Ref_get_accessor_declaration().(*csharpparser.Ref_get_accessor_declarationContext); g != nil {
			if ab, _ := g.Ref_accessor_body().(*csharpparser.Ref_accessor_bodyContext); ab != nil {
				b.declareAccessor(class, "get_"+name, isStatic, ssa.DetachAST(ab.Block()), nil, ssa.DetachAST(ab.Variable_reference()), nil, getterOptions)
			}
		}
	}
}

// declareAccessorFromBody 处理 accessor_body: block | '=>' expression ';' | ';'
// 纯 `;` 的 auto 访问器不生成方法。
func (b *singleFileBuilder) declareAccessorFromBody(class *ssa.Blueprint, name string, static bool, raw csharpparser.IAccessor_bodyContext, params func(), options ...csharpAccessorOptions) {
	ab, _ := raw.(*csharpparser.Accessor_bodyContext)
	if ab == nil {
		return
	}
	if ab.Block() == nil && ab.Expression() == nil {
		return
	}
	b.declareAccessor(class, name, static, ssa.DetachAST(ab.Block()), ssa.DetachAST(ab.Expression()), nil, params, options...)
}

// declareAccessor 把 getter/setter/indexer/event 访问器编译为 get_X / set_X 方法。
func (b *singleFileBuilder) declareAccessor(class *ssa.Blueprint, name string, static bool,
	block csharpparser.IBlockContext, expr csharpparser.IExpressionContext, ref csharpparser.IVariable_referenceContext, params func(), options ...csharpAccessorOptions) {
	option := csharpAccessorOptions{}
	if len(options) != 0 {
		option = options[0]
	}
	f := b.newMethodFunction(class, name, static)
	if b.constructors.registerMethodMetadata(class, name, static, f, option.parameters, option.override, option.hides) {
		b.registerBlueprintSourceMethod(class, name, static, f)
	}
	b.buildFunctionLazy(f, class, static, params, func() {
		switch {
		case block != nil:
			b.visitFunctionBodyBlock(block)
		case expr != nil:
			if v := b.VisitExpression(expr); !utils.IsNil(v) {
				b.EmitReturn([]ssa.Value{v})
			}
		case ref != nil:
			if v := b.VisitVariableReference(ref); !utils.IsNil(v) {
				b.EmitReturn([]ssa.Value{v})
			}
		}
	})
}

func (b *singleFileBuilder) VisitEventDeclaration(raw csharpparser.IEvent_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Event_declarationContext)
	if !ok || i == nil {
		return
	}
	isStatic := false
	for _, m := range i.AllEvent_modifier() {
		if mc, _ := m.(*csharpparser.Event_modifierContext); mc != nil && mc.KW_STATIC() != nil {
			isStatic = true
		}
	}
	typeCtx := ssa.DetachAST(i.Type_())
	if dc, _ := i.Variable_declarators().(*csharpparser.Variable_declaratorsContext); dc != nil {
		for _, d := range dc.AllVariable_declarator() {
			vd, _ := d.(*csharpparser.Variable_declaratorContext)
			if vd == nil {
				continue
			}
			name := identText(vd.Identifier())
			if name == "" {
				continue
			}
			b.declareFieldLike(class, name, isStatic, typeCtx, ssa.DetachAST(vd.Variable_initializer()), vd.Identifier())
		}
		return
	}
	name := memberNameText(i.Member_name())
	if name == "" {
		return
	}
	b.declareFieldLike(class, name, isStatic, typeCtx, nil, nil)
	if acc, _ := i.Event_accessor_declarations().(*csharpparser.Event_accessor_declarationsContext); acc != nil {
		valueParam := func() {
			p := b.NewParam("value")
			p.SetType(b.VisitType(typeCtx))
		}
		if add, _ := acc.Add_accessor_declaration().(*csharpparser.Add_accessor_declarationContext); add != nil {
			b.declareAccessor(class, "add_"+name, isStatic, ssa.DetachAST(add.Block()), nil, nil, valueParam)
		}
		if rm, _ := acc.Remove_accessor_declaration().(*csharpparser.Remove_accessor_declarationContext); rm != nil {
			b.declareAccessor(class, "remove_"+name, isStatic, ssa.DetachAST(rm.Block()), nil, nil, valueParam)
		}
	}
}

func (b *singleFileBuilder) VisitIndexerDeclaration(raw csharpparser.IIndexer_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Indexer_declarationContext)
	if !ok || i == nil {
		return
	}
	decl, _ := i.Indexer_declarator().(*csharpparser.Indexer_declaratorContext)
	if decl == nil {
		return
	}
	params := ssa.DetachAST(decl.Parameter_list())
	typeCtx := ssa.DetachAST(decl.Type_())
	isOverride, hides := indexerDispatchFlags(i.AllIndexer_modifier())
	getterMetadata := csharpParameterMetadata(params)
	setterMetadata := append([]csharpConstructorParameter(nil), getterMetadata...)
	valueTypeName := ""
	if typeCtx != nil {
		valueTypeName = typeCtx.GetText()
	}
	setterMetadata = append(setterMetadata, csharpConstructorParameter{name: "value", typeName: valueTypeName})
	getterOptions := csharpAccessorOptions{parameters: getterMetadata, override: isOverride, hides: hides}
	setterOptions := csharpAccessorOptions{parameters: setterMetadata, override: isOverride, hides: hides}
	getParams := func() { b.VisitParameterList(params) }
	setParams := func() {
		b.VisitParameterList(params)
		p := b.NewParam("value")
		p.SetType(b.VisitType(typeCtx))
	}
	if body, _ := i.Indexer_body().(*csharpparser.Indexer_bodyContext); body != nil {
		if body.Expression() != nil {
			b.declareAccessor(class, "get_Item", false, nil, ssa.DetachAST(body.Expression()), nil, getParams, getterOptions)
			return
		}
		if acc, _ := body.Accessor_declarations().(*csharpparser.Accessor_declarationsContext); acc != nil {
			if g, _ := acc.Get_accessor_declaration().(*csharpparser.Get_accessor_declarationContext); g != nil {
				b.declareAccessorFromBody(class, "get_Item", false, g.Accessor_body(), getParams, getterOptions)
			}
			if s, _ := acc.Set_accessor_declaration().(*csharpparser.Set_accessor_declarationContext); s != nil {
				b.declareAccessorFromBody(class, "set_Item", false, s.Accessor_body(), setParams, setterOptions)
			}
		}
		return
	}
	if rb, _ := i.Ref_indexer_body().(*csharpparser.Ref_indexer_bodyContext); rb != nil {
		if rb.Variable_reference() != nil {
			b.declareAccessor(class, "get_Item", false, nil, nil, ssa.DetachAST(rb.Variable_reference()), getParams, getterOptions)
		} else if g, _ := rb.Ref_get_accessor_declaration().(*csharpparser.Ref_get_accessor_declarationContext); g != nil {
			if ab, _ := g.Ref_accessor_body().(*csharpparser.Ref_accessor_bodyContext); ab != nil {
				b.declareAccessor(class, "get_Item", false, ssa.DetachAST(ab.Block()), nil, ssa.DetachAST(ab.Variable_reference()), getParams, getterOptions)
			}
		}
	}
}

// ---------------------------------------------------------------- operator

var csharpOperatorMethodNames = map[string]string{
	"+": "op_Addition", "-": "op_Subtraction", "*": "op_Multiply", "/": "op_Division", "%": "op_Modulus",
	"&": "op_BitwiseAnd", "|": "op_BitwiseOr", "^": "op_ExclusiveOr", "<<": "op_LeftShift", ">>": "op_RightShift",
	"==": "op_Equality", "!=": "op_Inequality", ">": "op_GreaterThan", "<": "op_LessThan",
	">=": "op_GreaterThanOrEqual", "<=": "op_LessThanOrEqual",
	"!": "op_LogicalNot", "~": "op_OnesComplement", "++": "op_Increment", "--": "op_Decrement",
	"true": "op_True", "false": "op_False",
}

func operatorMethodName(op string, unary bool) string {
	if unary {
		switch op {
		case "+":
			return "op_UnaryPlus"
		case "-":
			return "op_UnaryNegation"
		}
	}
	if name, ok := csharpOperatorMethodNames[op]; ok {
		return name
	}
	return "op_" + op
}

func (b *singleFileBuilder) VisitOperatorDeclaration(raw csharpparser.IOperator_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Operator_declarationContext)
	if !ok || i == nil {
		return
	}
	decl, _ := i.Operator_declarator().(*csharpparser.Operator_declaratorContext)
	if decl == nil {
		return
	}
	var (
		name   string
		params []csharpparser.IFixed_parameterContext
		retTyp csharpparser.IType_Context
	)
	switch {
	case decl.Unary_operator_declarator() != nil:
		u, _ := decl.Unary_operator_declarator().(*csharpparser.Unary_operator_declaratorContext)
		if u == nil {
			return
		}
		name = operatorMethodName(u.Overloadable_unary_operator().GetText(), true)
		params = []csharpparser.IFixed_parameterContext{u.Fixed_parameter()}
		retTyp = u.Type_()
	case decl.Binary_operator_declarator() != nil:
		bi, _ := decl.Binary_operator_declarator().(*csharpparser.Binary_operator_declaratorContext)
		if bi == nil {
			return
		}
		name = operatorMethodName(bi.Overloadable_binary_operator().GetText(), false)
		params = bi.AllFixed_parameter()
		retTyp = bi.Type_()
	case decl.Conversion_operator_declarator() != nil:
		c, _ := decl.Conversion_operator_declarator().(*csharpparser.Conversion_operator_declaratorContext)
		if c == nil {
			return
		}
		name = "op_Explicit"
		if c.KW_IMPLICIT() != nil {
			name = "op_Implicit"
		}
		params = []csharpparser.IFixed_parameterContext{c.Fixed_parameter()}
		retTyp = c.Type_()
	default:
		return
	}
	for idx := range params {
		params[idx] = ssa.DetachAST(params[idx])
	}
	retTyp = ssa.DetachAST(retTyp)
	body := ssa.DetachAST(i.Operator_body())
	metadata := make([]csharpConstructorParameter, 0, len(params))
	for _, rawParam := range params {
		param, _ := rawParam.(*csharpparser.Fixed_parameterContext)
		if param == nil {
			continue
		}
		item := csharpConstructorParameter{
			name:     identText(param.Identifier()),
			modifier: csharpFixedParameterModifier(param),
		}
		if param.Type_() != nil {
			item.typeName = param.Type_().GetText()
		}
		metadata = append(metadata, item)
	}
	f := b.newMethodFunction(class, name, true)
	if b.constructors.registerMethodMetadata(class, name, true, f, metadata, false, false) {
		b.registerBlueprintSourceMethod(class, name, true, f)
	}
	b.buildFunctionLazy(f, class, true, func() {
		if retTyp != nil {
			b.SetCurrentReturnType(b.VisitType(retTyp))
		}
		for _, p := range params {
			b.VisitFixedParameter(p)
		}
	}, func() {
		ob, _ := body.(*csharpparser.Operator_bodyContext)
		if ob == nil {
			return
		}
		if ob.Block() != nil {
			b.VisitBlock(ob.Block())
		} else if ob.Expression() != nil {
			if v := b.VisitExpression(ob.Expression()); !utils.IsNil(v) {
				b.EmitReturn([]ssa.Value{v})
			}
		}
	})
}

// ---------------------------------------------------------------- constructor / finalizer

func (b *singleFileBuilder) VisitConstructorDeclaration(raw csharpparser.IConstructor_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Constructor_declarationContext)
	if !ok || i == nil {
		return
	}
	decl, _ := i.Constructor_declarator().(*csharpparser.Constructor_declaratorContext)
	if decl == nil {
		return
	}
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, "ctor", uuid.NewString()[:4])
	f := b.NewFunc(funcName)
	f.SetMethodName(class.Name)
	b.ensureBlueprintConstructorSlot(class)
	firstConstructor := len(b.constructors.candidates(class)) == 0
	b.constructors.register(class, f, decl.Parameter_list())
	// Keep one constructor in the generic Blueprint slot for compatibility with
	// non-C# consumers. C# call sites use the registry above; registering every
	// overload as one magic method would Point them together and cross-contaminate
	// otherwise unrelated receiver state.
	if firstConstructor {
		class.RegisterMagicMethod(ssa.Constructor, f)
	}

	params := ssa.DetachAST(decl.Parameter_list())
	initializer := ssa.DetachAST(decl.Constructor_initializer())
	body := ssa.DetachAST(i.Constructor_body())
	b.lazyBuild(f.AddLazyBuilder, func() {
		b.FunctionBuilder = b.PushFunction(f)
		self := b.NewParam("$this")
		self.SetType(class)
		// Constructors operate on the instance supplied by the call site.  Using a
		// fresh container here would make an explicit base(...) initializer mutate
		// a disconnected Base object, so its fields could never reach the Child
		// returned to the caller.  Keeping `$this` as the constructor result also
		// lets the ordinary call-side-effect machinery project member writes back
		// onto the allocated instance.
		b.AssignVariable(b.CreateVariable("this"), self)
		b.MarkedThisClassBlueprint = class
		b.VisitParameterList(params)
		b.visitConstructorInitializer(initializer, class, self, f)
		if cb, _ := body.(*csharpparser.Constructor_bodyContext); cb != nil {
			if cb.Block() != nil {
				b.VisitBlock(cb.Block())
			} else if cb.Expression() != nil {
				b.VisitExpression(cb.Expression())
			}
		}
		b.EmitReturn([]ssa.Value{self})
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}

// visitConstructorInitializer 处理 `: base(...)` / `: this(...)`。
func (b *singleFileBuilder) visitConstructorInitializer(raw csharpparser.IConstructor_initializerContext, class *ssa.Blueprint, this ssa.Value, current *ssa.Function) {
	i, _ := raw.(*csharpparser.Constructor_initializerContext)
	if i == nil {
		// Every instance constructor without an explicit initializer invokes the
		// accessible parameterless/defaulted constructor of its direct base.
		if parent := class.GetSuperBlueprint(); parent != nil {
			b.emitConstructorCall(parent, this, nil, nil, true)
		}
		return
	}
	args := b.visitArgumentDetails(i.Argument_list())
	if i.KW_THIS() != nil {
		// Excluding the current function prevents a malformed/self-resolving
		// initializer from producing a recursive call. Legal chains select another
		// overload and keep the exact same `$this` receiver.
		b.emitConstructorCall(class, this, args, current, false)
		return
	}
	var target *ssa.Blueprint
	if i.KW_BASE() != nil {
		target = class.GetSuperBlueprint()
	}
	if target == nil {
		return
	}
	b.emitConstructorCall(target, this, args, nil, true)
}

func (b *singleFileBuilder) VisitStaticConstructorDeclaration(raw csharpparser.IStatic_constructor_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Static_constructor_declarationContext)
	if !ok || i == nil {
		return
	}
	body := ssa.DetachAST(i.Static_constructor_body())
	f := b.declareMethod(class, "cctor", true)
	b.buildFunctionLazy(f, class, true, nil, func() {
		sb, _ := body.(*csharpparser.Static_constructor_bodyContext)
		if sb == nil {
			return
		}
		if sb.Block() != nil {
			b.VisitBlock(sb.Block())
		} else if sb.Expression() != nil {
			b.VisitExpression(sb.Expression())
		}
	})
}

func (b *singleFileBuilder) VisitFinalizerDeclaration(raw csharpparser.IFinalizer_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Finalizer_declarationContext)
	if !ok || i == nil {
		return
	}
	body := ssa.DetachAST(i.Finalizer_body())
	f := b.declareMethod(class, "Finalize", false)
	b.buildFunctionLazy(f, class, false, nil, func() {
		fb, _ := body.(*csharpparser.Finalizer_bodyContext)
		if fb == nil {
			return
		}
		if fb.Block() != nil {
			b.VisitBlock(fb.Block())
		} else if fb.Expression() != nil {
			b.VisitExpression(fb.Expression())
		}
	})
}

// blueprintDisplayName 用于调试日志。
func blueprintDisplayName(bp *ssa.Blueprint) string {
	if bp == nil {
		return "<nil>"
	}
	if names := bp.GetFullTypeNames(); len(names) > 0 {
		return strings.Join(names, "|")
	}
	return bp.Name
}
