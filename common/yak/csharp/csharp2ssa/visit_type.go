package csharp2ssa

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// ensureBlueprintMemberSlot pre-declares a class member so later storeField
// (RegisterStaticMethod / RegisterNormalMethod / constructor magic method)
// does not emit ObjectError for a missing member. C# methods may share the
// type name (class Main { static void Main() } / constructors).
func (b *singleFileBuilder) ensureBlueprintMemberSlot(blueprint *ssa.Blueprint, name string, static bool) {
	if b == nil || blueprint == nil || name == "" {
		return
	}
	placeholder := b.EmitUndefined(name)
	if static {
		if utils.IsNil(blueprint.GetStaticMember(name)) {
			blueprint.RegisterStaticMember(name, placeholder, false)
		}
		return
	}
	if utils.IsNil(blueprint.GetNormalMember(name)) {
		blueprint.RegisterNormalMember(name, placeholder, false)
	}
}

func (b *singleFileBuilder) ensureBlueprintConstructorSlot(blueprint *ssa.Blueprint) {
	if blueprint == nil || blueprint.Name == "" {
		return
	}
	b.ensureBlueprintMemberSlot(blueprint, blueprint.Name, false)
	b.ensureBlueprintMemberSlot(blueprint, blueprint.Name, true)
}

func (b *singleFileBuilder) VisitClassDeclaration(raw csharpparser.IClass_declarationContext, out *ssa.Blueprint) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return b.EmitEmptyContainer()
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Class_declarationContext)
	if !ok || i == nil {
		return b.EmitEmptyContainer()
	}
	name := identText(i.Identifier())
	if out != nil {
		name = out.Name + "_" + name
	}
	blueprint := b.CreateBlueprint(name, i.Identifier())
	blueprint.SetKind(ssa.BlueprintClass)
	b.GetProgram().SetExportType(name, blueprint)
	b.ensureBlueprintConstructorSlot(blueprint)

	if cb := i.Class_base(); cb != nil {
		base, _ := cb.(*csharpparser.Class_baseContext)
		if base != nil {
			if ct := base.Class_type(); ct != nil {
				parentName := ct.GetText()
				store := b.StoreFunctionBuilder()
				blueprint.AddLazyBuilder(func() {
					switchHandler := b.SwitchFunctionBuilder(store)
					defer switchHandler()
					parent := b.GetBluePrint(parentName)
					if parent == nil {
						parent = b.CreateBlueprint(parentName, ct)
						parent.SetKind(ssa.BlueprintClass)
					}
					blueprint.AddParentBlueprint(parent)
				})
			}
			if list := base.Interface_type_list(); list != nil {
				for _, iface := range strings.Split(list.GetText(), ",") {
					iface = strings.TrimSpace(iface)
					if iface == "" {
						continue
					}
					iname := iface
					store := b.StoreFunctionBuilder()
					blueprint.AddLazyBuilder(func() {
						switchHandler := b.SwitchFunctionBuilder(store)
						defer switchHandler()
						parent := b.GetBluePrint(iname)
						if parent == nil {
							parent = b.CreateBlueprint(iname)
							parent.SetKind(ssa.BlueprintInterface)
						}
						blueprint.AddInterfaceBlueprint(parent)
					})
				}
			}
		}
	}

	container := blueprint.Container()
	prev := b.MarkedThisClassBlueprint
	b.MarkedThisClassBlueprint = blueprint
	defer func() { b.MarkedThisClassBlueprint = prev }()
	b.PushBlueprint(blueprint)
	defer b.PopBlueprint()
	if body := i.Class_body(); body != nil {
		bc, _ := body.(*csharpparser.Class_bodyContext)
		if bc != nil {
			for _, member := range bc.AllClass_member_declaration() {
				b.VisitClassMemberDeclaration(member, blueprint)
			}
		}
	}
	return container
}

func (b *singleFileBuilder) VisitStructDeclaration(raw csharpparser.IStruct_declarationContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return b.EmitEmptyContainer()
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Struct_declarationContext)
	if !ok || i == nil {
		return b.EmitEmptyContainer()
	}
	name := identText(i.Identifier())
	blueprint := b.CreateBlueprint(name, i.Identifier())
	blueprint.SetKind(ssa.BlueprintClass)
	b.GetProgram().SetExportType(name, blueprint)
	b.ensureBlueprintConstructorSlot(blueprint)
	b.PushBlueprint(blueprint)
	defer b.PopBlueprint()
	if body := i.Struct_body(); body != nil {
		bc, _ := body.(*csharpparser.Struct_bodyContext)
		if bc != nil {
			for _, member := range bc.AllStruct_member_declaration() {
				b.VisitStructMemberDeclaration(member, blueprint)
			}
		}
	}
	return blueprint.Container()
}

func (b *singleFileBuilder) VisitInterfaceDeclaration(raw csharpparser.IInterface_declarationContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return b.EmitEmptyContainer()
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Interface_declarationContext)
	if !ok || i == nil {
		return b.EmitEmptyContainer()
	}
	name := identText(i.Identifier())
	blueprint := b.CreateBlueprint(name, i.Identifier())
	blueprint.SetKind(ssa.BlueprintInterface)
	b.GetProgram().SetExportType(name, blueprint)
	b.ensureBlueprintConstructorSlot(blueprint)
	b.PushBlueprint(blueprint)
	defer b.PopBlueprint()
	if body := i.Interface_body(); body != nil {
		bc, _ := body.(*csharpparser.Interface_bodyContext)
		if bc != nil {
			for _, member := range bc.AllInterface_member_declaration() {
				mc, _ := member.(*csharpparser.Interface_member_declarationContext)
				if mc == nil {
					continue
				}
				if mc.Method_declaration() != nil {
					b.VisitMethodDeclaration(mc.Method_declaration(), blueprint)
				} else if mc.Field_declaration() != nil {
					b.VisitFieldDeclaration(mc.Field_declaration(), blueprint)
				}
			}
		}
	}
	return blueprint.Container()
}

func (b *singleFileBuilder) VisitEnumDeclaration(raw csharpparser.IEnum_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Enum_declarationContext)
	if !ok || i == nil {
		return
	}
	name := identText(i.Identifier())
	if name == "" {
		return
	}
	blueprint := b.CreateBlueprint(name, i.Identifier())
	blueprint.SetKind(ssa.BlueprintClass)
	obj := b.EmitMakeWithoutType(nil, nil)
	obj.SetType(blueprint)
	b.AssignVariable(b.CreateVariable(name), obj)
}

func (b *singleFileBuilder) VisitClassMemberDeclaration(raw csharpparser.IClass_member_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Class_member_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Method_declaration() != nil:
		b.VisitMethodDeclaration(i.Method_declaration(), class)
	case i.Field_declaration() != nil:
		b.VisitFieldDeclaration(i.Field_declaration(), class)
	case i.Constructor_declaration() != nil:
		b.VisitConstructorDeclaration(i.Constructor_declaration(), class)
	case i.Constant_declaration() != nil:
		b.VisitConstantDeclaration(i.Constant_declaration(), class)
	case i.Type_declaration() != nil:
		b.VisitTypeDeclaration(i.Type_declaration())
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
	case i.Method_declaration() != nil:
		b.VisitMethodDeclaration(i.Method_declaration(), class)
	case i.Field_declaration() != nil:
		b.VisitFieldDeclaration(i.Field_declaration(), class)
	case i.Constructor_declaration() != nil:
		b.VisitConstructorDeclaration(i.Constructor_declaration(), class)
	case i.Type_declaration() != nil:
		b.VisitTypeDeclaration(i.Type_declaration())
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
		mc, _ := mod.(*csharpparser.Field_modifierContext)
		if mc != nil && mc.KW_STATIC() != nil {
			isStatic = true
		}
	}
	setMember := class.RegisterNormalMember
	if isStatic {
		setMember = class.RegisterStaticMember
	}
	decls := i.Variable_declarators()
	if decls == nil {
		return
	}
	dc, _ := decls.(*csharpparser.Variable_declaratorsContext)
	if dc == nil {
		return
	}
	for _, d := range dc.AllVariable_declarator() {
		vd, _ := d.(*csharpparser.Variable_declaratorContext)
		if vd == nil {
			continue
		}
		name := identText(vd.Identifier())
		if name == "" {
			continue
		}
		undefined := b.EmitUndefined(name)
		setMember(name, undefined, false)
		captured := ssa.DetachAST(d)
		store := b.StoreFunctionBuilder()
		class.AddLazyBuilder(func() {
			switchHandler := b.SwitchFunctionBuilder(store)
			defer switchHandler()
			cvd, _ := captured.(*csharpparser.Variable_declaratorContext)
			if cvd == nil {
				return
			}
			var value ssa.Value
			if init := cvd.Variable_initializer(); init != nil {
				value = b.VisitVariableInitializer(init)
			}
			if value == nil {
				return
			}
			setMember(name, value)
		})
	}
}

func (b *singleFileBuilder) VisitConstantDeclaration(raw csharpparser.IConstant_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Constant_declarationContext)
	if !ok || i == nil || i.Constant_declarators() == nil {
		return
	}
	dc, _ := i.Constant_declarators().(*csharpparser.Constant_declaratorsContext)
	if dc == nil {
		return
	}
	for _, d := range dc.AllConstant_declarator() {
		cd, _ := d.(*csharpparser.Constant_declaratorContext)
		if cd == nil {
			continue
		}
		name := identText(cd.Identifier())
		if name == "" {
			continue
		}
		var value ssa.Value
		if cd.Constant_expression() != nil {
			value = b.VisitExpression(cd.Constant_expression().Expression())
		}
		if value == nil {
			value = b.EmitUndefined(name)
		}
		class.RegisterStaticMember(name, value)
	}
}

func (b *singleFileBuilder) VisitMethodDeclaration(raw csharpparser.IMethod_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Method_declarationContext)
	if !ok || i == nil || i.Method_header() == nil {
		return
	}
	header, _ := i.Method_header().(*csharpparser.Method_headerContext)
	if header == nil || header.Member_name() == nil {
		return
	}
	mn, _ := header.Member_name().(*csharpparser.Member_nameContext)
	if mn == nil {
		return
	}
	key := identText(mn.Identifier())
	if key == "" {
		return
	}
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, key, uuid.NewString()[:4])
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(key)
	isStatic := methodIsStatic(i.Method_modifiers())
	b.ensureBlueprintMemberSlot(class, key, isStatic)
	if isStatic {
		class.RegisterStaticMethod(key, newFunc)
	} else {
		class.RegisterNormalMethod(key, newFunc)
	}
	params := ssa.DetachAST(header.Parameter_list())
	body := ssa.DetachAST(i.Method_body())
	refBody := ssa.DetachAST(i.Ref_method_body())
	store := b.StoreFunctionBuilder()
	newFunc.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)
		if !isStatic {
			this := b.NewParam("this")
			this.SetType(class)
		}
		b.MarkedThisClassBlueprint = class
		b.VisitParameterList(params)
		if body != nil {
			b.VisitMethodBody(body)
		} else if refBody != nil {
			rb, _ := refBody.(*csharpparser.Ref_method_bodyContext)
			if rb != nil && rb.Block() != nil {
				b.VisitBlock(rb.Block())
			}
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}

func (b *singleFileBuilder) VisitConstructorDeclaration(raw csharpparser.IConstructor_declarationContext, class *ssa.Blueprint) {
	if b == nil || raw == nil || class == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Constructor_declarationContext)
	if !ok || i == nil || i.Constructor_declarator() == nil {
		return
	}
	decl, _ := i.Constructor_declarator().(*csharpparser.Constructor_declaratorContext)
	if decl == nil {
		return
	}
	key := identText(decl.Identifier())
	if key == "" {
		key = class.Name
	}
	funcName := fmt.Sprintf("%s_ctor_%s", class.Name, uuid.NewString()[:4])
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(key)
	b.ensureBlueprintConstructorSlot(class)
	class.RegisterMagicMethod(ssa.Constructor, newFunc)
	params := ssa.DetachAST(decl.Parameter_list())
	body := ssa.DetachAST(i.Constructor_body())
	store := b.StoreFunctionBuilder()
	newFunc.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)
		this := b.NewParam("this")
		this.SetType(class)
		b.MarkedThisClassBlueprint = class
		b.VisitParameterList(params)
		if body != nil {
			cb, _ := body.(*csharpparser.Constructor_bodyContext)
			if cb != nil {
				if cb.Block() != nil {
					b.VisitBlock(cb.Block())
				} else if cb.Expression() != nil {
					b.VisitExpression(cb.Expression())
				}
			}
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
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
	if i.Block() != nil {
		b.VisitBlock(i.Block())
		return
	}
	if i.Expression() != nil {
		v := b.VisitExpression(i.Expression())
		if v != nil {
			b.EmitReturn([]ssa.Value{v})
		}
	}
}

func (b *singleFileBuilder) VisitParameterList(raw csharpparser.IParameter_listContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Parameter_listContext)
	if !ok || i == nil {
		return
	}
	if fp := i.Fixed_parameters(); fp != nil {
		fpc, _ := fp.(*csharpparser.Fixed_parametersContext)
		if fpc != nil {
			for _, p := range fpc.AllFixed_parameter() {
				pc, _ := p.(*csharpparser.Fixed_parameterContext)
				if pc == nil {
					continue
				}
				name := identText(pc.Identifier())
				if name == "" {
					continue
				}
				param := b.NewParam(name)
				if pc.Type_() != nil {
					param.SetType(b.VisitType(pc.Type_()))
				}
			}
		}
	}
}

func (b *singleFileBuilder) VisitType(raw csharpparser.IType_Context) ssa.Type {
	if b == nil || raw == nil {
		return ssa.CreateAnyType()
	}
	text := raw.GetText()
	switch text {
	case "bool":
		return ssa.CreateBooleanType()
	case "byte", "sbyte":
		return ssa.CreateByteType()
	case "char", "string":
		return ssa.CreateStringType()
	case "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "decimal":
		return ssa.CreateNumberType()
	case "void", "object", "dynamic", "var":
		return ssa.CreateAnyType()
	}
	if bp := b.GetBluePrint(text); bp != nil {
		return bp
	}
	bp := b.CreateBlueprint(text, raw)
	b.ensureBlueprintConstructorSlot(bp)
	return bp
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
		return b.EmitMakeWithoutType(nil, nil)
	}
	return nil
}

func methodIsStatic(raw csharpparser.IMethod_modifiersContext) bool {
	if raw == nil {
		return false
	}
	i, ok := raw.(*csharpparser.Method_modifiersContext)
	if !ok || i == nil {
		return false
	}
	for _, m := range i.AllMethod_modifier() {
		mc, _ := m.(*csharpparser.Method_modifierContext)
		if mc == nil {
			continue
		}
		if ref := mc.Ref_method_modifier(); ref != nil {
			rm, _ := ref.(*csharpparser.Ref_method_modifierContext)
			if rm != nil && rm.KW_STATIC() != nil {
				return true
			}
		}
	}
	return false
}
