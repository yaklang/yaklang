package java2ssa

import (
	"fmt"
	"github.com/google/uuid"
	"strings"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeDeclaration(raw javaparser.ITypeDeclarationContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.TypeDeclarationContext)
	if i == nil {
		return
	}
	type callback func(ssa.Value)

	var callBacks []callback

	var modifier []string
	for _, mod := range i.AllClassOrInterfaceModifier() {
		raw, ok := mod.(*javaparser.ClassOrInterfaceModifierContext)
		if !ok {
			continue
		}
		if raw.Annotation() != nil {
			instanceCallback, defCallback := y.VisitAnnotation(raw.Annotation())
			callBacks = append(callBacks, instanceCallback)
			callBacks = append(callBacks, defCallback)
		}
		modifier = append(modifier, mod.GetText())
	}
	if ret := i.ClassDeclaration(); ret != nil {
		container := y.VisitClassDeclaration(ret, nil)
		if container != nil {
			for _, callBack := range callBacks {
				callBack(container)
			}
		}
	} else if ret := i.EnumDeclaration(); ret != nil {
		y.VisitEnumDeclaration(ret, nil)
	} else if ret := i.InterfaceDeclaration(); ret != nil {
		container := y.VisitInterfaceDeclaration(ret)
		if container != nil {
			for _, callBack := range callBacks {
				callBack(container)
			}
		} else {
			log.Error("BUG: interface container is nil")
		}
	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {
		y.VisitAnnotationTypeDeclaration(ret)
	} else if ret := i.RecordDeclaration(); ret != nil {
		y.VisitRecordDeclaration(ret)
	}

}

func (y *builder) VisitClassDeclaration(raw javaparser.IClassDeclarationContext, outClass *ssa.Blueprint) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return y.EmitEmptyContainer()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassDeclarationContext)
	if i == nil {
		return y.EmitEmptyContainer()
	}
	var mergedTemplate []string
	// 声明的类为外部类情况
	var class *ssa.Blueprint
	if outClass == nil {
		className := i.Identifier().GetText()
		class = y.CreateBluePrint(className)
		y.GetProgram().SetExportType(className, class)
	} else {
		var builder strings.Builder
		builder.WriteString(outClass.Name)
		builder.WriteString(".")
		builder.WriteString(i.Identifier().GetText())
		className := builder.String()
		class = y.CreateBluePrint(className)
	}
	// set full type name for class's self
	if len(y.selfPkgPath) != 0 {
		ftRaw := fmt.Sprintf("%s.%s", strings.Join(y.selfPkgPath[:len(y.selfPkgPath)-1], "."), class.Name)
		class = y.AddFullTypeNameRaw(ftRaw, class).(*ssa.Blueprint)
	}
	if ret := i.TypeParameters(); ret != nil {
		//log.Infof("class: %v 's (generic type) type is %v, ignore for ssa building", className, ret.GetText())
	}

	var classContainerCallback []func(ssa.Value)
	var classlessParents []string
	if i.EXTENDS() != nil {
		if extend := i.TypeType(); extend != nil {
			parentName := extend.GetText()
			classContainerCallback = append(classContainerCallback, func(value ssa.Value) {
				variable := y.CreateMemberCallVariable(value, y.EmitConstInst("extends"))
				y.AssignVariable(variable, y.EmitConstInst(parentName))
				classlessParents = append(classlessParents, parentName)
			})
			mergedTemplate = append(mergedTemplate, parentName)
		}
	}

	//haveImplements := false
	if i.IMPLEMENTS() != nil {
		//haveImplements = true
		var implName []string
		for _, val := range i.AllTypeList() {
			implName = append(implName, val.GetText())
			classlessParents = append(classlessParents, val.GetText())
		}
		if len(implName) > 0 {
			classContainerCallback = append(classContainerCallback, func(value ssa.Value) {
				variable := y.CreateMemberCallVariable(value, y.EmitConstInst("implements"))
				y.AssignVariable(variable, y.EmitConstInst(strings.Join(implName, ",")))
			})
		}
		mergedTemplate = append(mergedTemplate, i.TypeList(0).GetText())
	}

	classlessParents = utils.StringArrayFilterEmpty(classlessParents)
	if len(classlessParents) > 0 {
		classContainerCallback = append(classContainerCallback, func(value ssa.Value) {
			variable := y.CreateMemberCallVariable(value, y.EmitConstInst("inherits"))
			y.AssignVariable(variable, y.EmitConstInst(strings.Join(classlessParents, ",")))
		})
	}
	/*
		该lazyBuilder顺序按照cls解析顺序
	*/
	current := y.FunctionBuilder
	currentEditor := y.FunctionBuilder.GetEditor()
	class.AddLazyBuilder(func() {
		f := y.SwitchProg(current, currentEditor)
		defer f()
		for _, parentClass := range mergedTemplate {
			if bluePrint := y.GetBluePrint(parentClass); bluePrint != nil {
				class.AddParentClass(bluePrint)
			} else {
				parentX := y.CreateBluePrint(parentClass)
				y.AddFullTypeNameForAllImport(parentClass, parentX)
				class.AddParentClass(parentX)
			}
		}
	})
	container := class.GetClassContainer()
	y.VisitClassBody(i.ClassBody(), class)
	for _, callback := range classContainerCallback {
		callback(container)
	}
	return container
}

func (y *builder) VisitClassBody(raw javaparser.IClassBodyContext, class *ssa.Blueprint) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassBodyContext)
	if i == nil {
		return nil
	}

	y.PushBluePrint(class)
	defer y.PopBluePrint()

	for _, ret := range i.AllClassBodyDeclaration() {
		y.VisitClassBodyDeclaration(ret, class)
	}
	return nil
}

func (y *builder) VisitFormalParameters(raw javaparser.IFormalParametersContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.FormalParametersContext)
	if i == nil {
		return
	}

	if i.ReceiverParameter() != nil && i.COMMA() == nil {
		y.VisitReceiverParameter(i.ReceiverParameter())
	} else if i.ReceiverParameter() != nil && i.COMMA() != nil {
		y.VisitReceiverParameter(i.ReceiverParameter())
		y.VisitFormalParameterList(i.FormalParameterList())
	} else if i.FormalParameterList() != nil && i.COMMA() == nil {
		y.VisitFormalParameterList(i.FormalParameterList())
	}

}

func (y *builder) VisitMemberDeclaration(raw javaparser.IMemberDeclarationContext, modifiers javaparser.IModifiersContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.MemberDeclarationContext)
	if i == nil {
		return
	}
	annotationFunc, defCallbacks, isStatic := y.VisitModifiers(modifiers)
	_ = annotationFunc
	_ = defCallbacks
	if i.ConstructorDeclaration() != nil {
		y.VisitConstructorDeclaration(i.ConstructorDeclaration(), class)
	} else if i.FieldDeclaration() != nil {
		currentBuilder := y.FunctionBuilder
		currentEditor := y.FunctionBuilder.GetEditor()
		setMember := class.RegisterNormalMember
		if isStatic {
			setMember = class.RegisterStaticMember
		}
		field := i.FieldDeclaration().(*javaparser.FieldDeclarationContext)
		variableDeclarators := field.VariableDeclarators().(*javaparser.VariableDeclaratorsContext).AllVariableDeclarator()
		for _, name := range variableDeclarators {
			namex := y.OnlyVisitVariableDeclaratorName(name)
			undefined := ssa.Value(ssa.NewUndefined(namex))
			setMember(namex, undefined, false)
		}
		class.AddLazyBuilder(func() {
			f := y.SwitchProg(currentBuilder, currentEditor)
			defer f()
			var fieldType ssa.Type
			if field.TypeType() != nil {
				typex := field.TypeType().GetText()
				_ = typex
				fieldType = y.VisitTypeType(field.TypeType())
			}
			_ = fieldType
			for _, variableDeclarator := range variableDeclarators {
				v := variableDeclarator.(*javaparser.VariableDeclaratorContext)
				name, value := y.VisitVariableDeclarator(v, nil)
				value.SetType(fieldType)
				setMember(name, value)
			}
		})
	} else if ret := i.RecordDeclaration(); ret != nil {
		log.Infof("todo: java17: %v", ret.GetText())
	} else if ret := i.MethodDeclaration(); ret != nil {
		y.VisitMethodDeclaration(ret, class, isStatic, annotationFunc, defCallbacks)
	} else if ret := i.GenericMethodDeclaration(); ret != nil {
	} else if ret := i.GenericConstructorDeclaration(); ret != nil {

	} else if ret := i.InterfaceDeclaration(); ret != nil {

	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {

	} else if ret := i.ClassDeclaration(); ret != nil {
		y.VisitClassDeclaration(ret, class)
	} else if ret := i.EnumDeclaration(); ret != nil {
		// 声明枚举类型
		y.VisitEnumDeclaration(ret, class)

	} else {
		log.Errorf("no member declaration found: %v", i.GetText())
	}

	return
}
func (y *builder) VisitTypeType(raw javaparser.ITypeTypeContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.TypeTypeContext)
	if i == nil {
		return nil
	}

	//log.Infof("start to handle type type: %v", i.GetText())
	for _, annotation := range i.AllAnnotation() {
		y.VisitAnnotation(annotation)
	}

	var t ssa.Type
	if ret := i.ClassOrInterfaceType(); ret != nil {
		t = y.VisitClassOrInterfaceType(ret)
	} else {
		t = y.VisitPrimitiveType(i.PrimitiveType())
	}

	return t
}

func (y *builder) VisitClassOrInterfaceType(raw javaparser.IClassOrInterfaceTypeContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	// log.Infof("class/interface: %v", raw.ToStringTree(raw.GetParser().GetRuleNames(), raw.GetParser()))
	// todo 类和接口的类型声明
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassOrInterfaceTypeContext)
	if i == nil {
		return nil
	}
	// if len(i.AllIdentifier()) == 1 {
	// 	// only one type
	var typ ssa.Type
	className := i.TypeIdentifier().GetText()
	//wrapper class
	switch className {
	case "Boolean":
		typ = ssa.CreateBooleanType()
		typ.AddFullTypeName(className)
		return typ
	case "Byte":
		typ = ssa.CreateBytesType()
		typ.AddFullTypeName(className)
		return typ
	case "Integer", "Long", "Float", "Double":
		typ = ssa.CreateNumberType()
		typ.AddFullTypeName(className)
		return typ
	case "String", "Character":
		typ = ssa.CreateStringType()
		typ.AddFullTypeName(className)
		return typ
	}
	if class := y.GetBluePrint(className); class != nil {
		typ = class
		if len(typ.GetFullTypeNames()) == 0 {
			typ = y.AddFullTypeNameFromMap(className, typ)
		}
		return typ
	} else {
		typ = ssa.NewClassBluePrint(className)
		typ = y.AddFullTypeNameFromMap(className, typ)
		return typ
	}
}

func (y *builder) VisitPrimitiveType(raw javaparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}
	var t ssa.Type
	switch i.GetText() {
	case "boolean":
		t = ssa.CreateBooleanType()
	case "char", "short", "int", "long", "float", "double":
		t = ssa.CreateNumberType()
	case "byte":
		t = ssa.CreateByteType()
	default:
		t = ssa.CreateAnyType()
	}
	if text := i.GetText(); text != "" {
		t.AddFullTypeName(text)
	} else {
		t.AddFullTypeName(t.String())
	}
	return t
}

func (y *builder) VisitEnumDeclaration(raw javaparser.IEnumDeclarationContext, class *ssa.Blueprint) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.EnumDeclarationContext)
	if i == nil {
		return nil
	}

	var mergedTemplate []string

	enumName := i.Identifier().GetText()
	if class == nil {
		class = y.CreateBluePrint(enumName)
	}

	if i.IMPLEMENTS() != nil {
		mergedTemplate = append(mergedTemplate, i.TypeList().GetText())
	}

	for _, parentClass := range mergedTemplate {
		if parent := y.GetBluePrint(parentClass); parent != nil {
			class.AddParentClass(parent)
		} else {
			class.AddParentClass(y.CreateBluePrint(parentClass))
		}
	}

	if i.EnumBodyDeclarations() != nil {
		y.VisitEnumBodyDeclarations(i.EnumBodyDeclarations(), class)
	}

	if i.EnumConstants() != nil {
		y.VisitEnumConstants(i.EnumConstants(), class)
	}
	// 将enum实例化并设置为全局变量
	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(class)
	variable := y.CreateVariable(enumName)
	y.AssignVariable(variable, obj)
	y.AssignConst(enumName, obj)

	return nil
}

func (y *builder) VisitEnumConstants(raw javaparser.IEnumConstantsContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.EnumConstantsContext)
	if i == nil {
		return
	}
	allEnumConstant := i.AllEnumConstant()
	for _, enumConstant := range allEnumConstant {
		y.VisitEnumConstant(enumConstant, class)
	}

	// 实例化enum里的常量
	obj := y.EmitMakeWithoutType(nil, nil)
	obj.SetType(class)
	setMember := class.RegisterNormalMember
	for _, enumConstant := range allEnumConstant {
		constant := enumConstant.(*javaparser.EnumConstantContext)
		enumName := constant.Identifier().GetText()
		arguments := constant.Arguments()
		constructor := class.Constructor
		if constructor == nil {
			setMember(enumName, obj)
		} else {
			args := []ssa.Value{obj}
			arguments := y.VisitArguments(arguments)
			args = append(args, arguments...)
			c := y.NewCall(constructor, args)
			y.EmitCall(c)
			setMember(enumName, obj)
		}

	}

}

func (y *builder) VisitEnumConstant(raw javaparser.IEnumConstantContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.EnumConstantContext)
	if i == nil {
		return
	}

	for _, annotation := range i.AllAnnotation() {
		_ = annotation
	}

	setMember := class.RegisterStaticMember

	name := i.Identifier().GetText()
	variable := y.CreateVariable(name)
	_ = variable
	setMember(name, y.EmitValueOnlyDeclare(name))
	return
}

func (y *builder) VisitEnumBodyDeclarations(raw javaparser.IEnumBodyDeclarationsContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	i, _ := raw.(*javaparser.EnumBodyDeclarationsContext)
	if i == nil {
		return
	}

	for _, ret := range i.AllClassBodyDeclaration() {
		y.VisitClassBodyDeclaration(ret, class)
	}
}

func (y *builder) VisitClassBodyDeclaration(
	raw javaparser.IClassBodyDeclarationContext,
	class *ssa.Blueprint,
) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}

	i, _ := raw.(*javaparser.ClassBodyDeclarationContext)
	if i == nil {
		return
	}

	if ret := i.Block(); ret != nil {
		currentFb := y.FunctionBuilder
		currentEditor := y.FunctionBuilder.GetEditor()
		class.AddLazyBuilder(func() {
			f := y.SwitchProg(currentFb, currentEditor)
			y.VisitBlock(i.Block())
			f()
		})
	} else if ret := i.MemberDeclaration(); ret != nil {
		if class != nil {
			y.VisitMemberDeclaration(ret, i.Modifiers(), class)
		}
	}
	return
}

func (y *builder) VisitAnnotationTypeDeclaration(raw javaparser.IAnnotationTypeDeclarationContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.AnnotationTypeDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitRecordDeclaration(raw javaparser.IRecordDeclarationContext) (string, []ssa.Value) {
	if y == nil || raw == nil || y.IsStop() {
		return "", nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.RecordDeclarationContext)
	if i == nil {
		return "", nil
	}

	return i.Identifier().GetText(), []ssa.Value{
		y.EmitConstInst(i.GetText()),
	}
}

func (y *builder) VisitMethodDeclaration(
	raw javaparser.IMethodDeclarationContext,
	class *ssa.Blueprint, isStatic bool,
	annotationFunc []func(ssa.Value),
	defCallback []func(ssa.Value),
) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.MethodDeclarationContext)
	if i == nil {
		return
	}

	key := i.Identifier().GetText()
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, key, uuid.NewString()[:4])
	methodName := key
	newFunc := y.NewFunc(funcName)
	newFunc.SetMethodName(methodName)
	if isStatic {
		class.RegisterStaticMethod(key, newFunc)
	} else {
		class.RegisterNormalMethod(key, newFunc)
	}
	currentBuilder := y.FunctionBuilder
	currentEditor := y.FunctionBuilder.GetEditor()
	newFunc.AddLazyBuilder(func() {
		f := y.SwitchProg(currentBuilder, currentEditor)
		defer f()
		y.FunctionBuilder = y.PushFunction(newFunc)
		if isStatic {
			y.SetType(y.VisitTypeTypeOrVoid(i.TypeTypeOrVoid()))
		}
		if !isStatic {
			this := y.NewParam("this", raw)
			this.SetType(class)
		}
		y.MarkedThisClassBlueprint = class
		y.SetCurrentReturnType(y.VisitTypeTypeOrVoid(i.TypeTypeOrVoid()))
		y.VisitFormalParameters(i.FormalParameters())
		y.VisitMethodBody(i.MethodBody())
		y.Finish()
		y.FunctionBuilder = y.PopFunction()
		if len(annotationFunc) > 0 || len(defCallback) > 0 {
			log.Infof("start to build annotation ref to def: %v", funcName)
		}
		newFunc.Type.AddAnnotationFunc(annotationFunc...)
		for _, def := range defCallback {
			def(newFunc)
		}
	})
	return
}

func (y *builder) VisitMethodBody(raw javaparser.IMethodBodyContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.MethodBodyContext)
	if i == nil {
		return
	}

	y.VisitBlock(i.Block())
}

func (y *builder) VisitTypeTypeOrVoid(raw javaparser.ITypeTypeOrVoidContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.TypeTypeOrVoidContext)
	if i == nil {
		return nil
	}
	if ret := i.TypeType(); ret != nil {
		return y.VisitTypeType(ret)
	} else {
		return ssa.CreateAnyType()
	}

}

func (y *builder) VisitFormalParameterList(raw javaparser.IFormalParameterListContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.FormalParameterListContext)
	if i == nil {
		return
	}

	if allFormalParam := i.AllFormalParameter(); allFormalParam != nil {
		for _, param := range allFormalParam {
			y.VisitFormalParameter(param)
		}
		if lastFormalParam := i.LastFormalParameter(); lastFormalParam != nil {
			y.VisitLastFormalParameter(lastFormalParam)
		}
	} else {
		if lastFormalParam := i.LastFormalParameter(); lastFormalParam != nil {
			y.VisitLastFormalParameter(lastFormalParam)
		}
	}

}

func (y *builder) VisitReceiverParameter(raw javaparser.IReceiverParameterContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ReceiverParameterContext)
	if i == nil {
		return
	}

	typeType := y.VisitTypeType(i.TypeType())
	_ = typeType
	// todo 接口的形参
}

func (y *builder) VisitFormalParameter(raw javaparser.IFormalParameterContext) (typeCallbacks, insCallbacks []func(ssa.Value)) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.FormalParameterContext)
	if i == nil {
		return
	}
	for _, modifier := range i.AllVariableModifier() {
		typeCallback, insCallback := y.VisitVariableModifier(modifier)
		typeCallbacks = append(typeCallbacks, typeCallback)
		insCallbacks = append(insCallbacks, insCallback)
	}
	typeType := y.VisitTypeType(i.TypeType())
	formalParams := y.VisitVariableDeclaratorId(i.VariableDeclaratorId())
	param := y.NewParam(formalParams)
	if typeType != nil {
		param.SetType(typeType)
	}

	if len(typeCallbacks) > 0 || len(insCallbacks) > 0 {
		log.Infof("start to apply annotation to formal-param: %v", param.String())

		if typeType != nil {
			for _, callback := range typeCallbacks {
				_ = callback
				log.Warn("TBD: treat type callback plz")
			}
		}
		for _, callback := range insCallbacks {
			callback(param)
		}
	}

	return
}

func (y *builder) VisitVariableDeclaratorId(raw javaparser.IVariableDeclaratorIdContext) string {
	if y == nil || raw == nil || y.IsStop() {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.VariableDeclaratorIdContext)
	if i == nil {
		return ""
	}
	text := i.Identifier().GetText()
	if text == "" {
		return ""
	}
	y.CreateVariable(text)
	return text
}

func (y *builder) VisitLastFormalParameter(raw javaparser.ILastFormalParameterContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LastFormalParameterContext)
	if i == nil {
		return
	}

	for _, modifier := range i.AllVariableModifier() {
		y.VisitVariableModifier(modifier)
	}

	for _, annotation := range i.AllAnnotation() {
		//todo annotation
		log.Warn("TBD: Annotation in VisitLastFormalParameter")
		log.Warn("TBD: Annotation in VisitLastFormalParameter")
		log.Warn("TBD: Annotation in VisitLastFormalParameter")
		log.Warn("TBD: Annotation in VisitLastFormalParameter")
		log.Warn("TBD: Annotation in VisitLastFormalParameter")
		_ = annotation
		//y.VisitAnnotation(annotation)
	}
	formalParams := y.VisitVariableDeclaratorId(i.VariableDeclaratorId())
	typeType := y.VisitTypeType(i.TypeType())
	isVariadic := i.ELLIPSIS()
	_ = isVariadic
	param := y.NewParam(formalParams)
	if typeType != nil {
		param.SetType(typeType)
	}
}

func (y *builder) VisitVariableModifier(raw javaparser.IVariableModifierContext) (typeCallback, insCallback func(ssa.Value)) {
	typeCallback = func(ssa.Value) {}
	insCallback = func(ssa.Value) {}

	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.VariableModifierContext)
	if i == nil {
		return
	}

	if i.FINAL() != nil {
		log.Info("VariableModifier: FINAL is ignored by ssa")
		return
	}
	return y.VisitAnnotation(i.Annotation())
}

func (y *builder) VisitQualifiedNameList(raw javaparser.IQualifiedNameListContext) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.QualifiedNameListContext)
	if i == nil {
		return
	}

}

func (y *builder) VisitConstructorDeclaration(raw javaparser.IConstructorDeclarationContext, class *ssa.Blueprint) {
	if y == nil || raw == nil || y.IsStop() {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ConstructorDeclarationContext)
	if i == nil {
		return
	}

	key := i.Identifier().GetText()
	pkgName := y.GetProgram()
	funcName := fmt.Sprintf("%s_%s_%s_%s", pkgName.Name, class.Name, key, uuid.NewString()[:4])
	newFunc := y.NewFunc(funcName)
	class.Constructor = newFunc
	currentBuilder := y.FunctionBuilder
	currentEditor := y.FunctionBuilder.GetEditor()
	class.RegisterMagicMethod(ssa.Constructor, newFunc)
	newFunc.AddLazyBuilder(func() {
		f := y.SwitchProg(currentBuilder, currentEditor)
		defer f()
		y.FunctionBuilder = y.PushFunction(newFunc)
		{
			y.NewParam("$this")
			container := y.EmitEmptyContainer()
			variable := y.CreateVariable("this")
			y.AssignVariable(variable, container)
			container.SetType(class)
			y.VisitFormalParameters(i.FormalParameters())
			y.VisitBlock(i.Block())
			y.EmitReturn([]ssa.Value{container})
			y.Finish()
		}
		if i.THROWS() != nil {
			y.VisitQualifiedNameList(i.QualifiedNameList())
		}
		y.FunctionBuilder = y.PopFunction()
	})
}
