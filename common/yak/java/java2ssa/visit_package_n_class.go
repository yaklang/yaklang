package java2ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeDeclaration(raw javaparser.ITypeDeclarationContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.TypeDeclarationContext)
	if i == nil {
		return
	}

	var modifier []string
	for _, mod := range i.AllClassOrInterfaceModifier() {
		modifier = append(modifier, mod.GetText())
	}

	if ret := i.ClassDeclaration(); ret != nil {
		y.VisitClassDeclaration(ret, nil)
	} else if ret := i.EnumDeclaration(); ret != nil {
		y.VisitEnumDeclaration(ret, nil)
	} else if ret := i.InterfaceDeclaration(); ret != nil {
		y.VisitInterfaceDeclaration(ret)
	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {
		y.VisitAnnotationTypeDeclaration(ret)
	} else if ret := i.RecordDeclaration(); ret != nil {
		y.VisitRecordDeclaration(ret)
	}

}

func (y *builder) VisitClassDeclaration(raw javaparser.IClassDeclarationContext, outClass *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassDeclarationContext)
	if i == nil {
		return
	}
	var mergedTemplate []string
	// 声明的类为外部类情况
	var class *ssa.ClassBluePrint
	if outClass == nil {
		className := i.Identifier().GetText()
		class = y.CreateClassBluePrint(className)
	} else {
		var builder strings.Builder
		builder.WriteString(outClass.Name)
		builder.WriteString(".")
		builder.WriteString(i.Identifier().GetText())
		className := builder.String()
		class = y.CreateClassBluePrint(className)
	}
	if ret := i.TypeParameters(); ret != nil {
		//log.Infof("class: %v 's (generic type) type is %v, ignore for ssa building", className, ret.GetText())
	}

	if extend := i.TypeType(); extend != nil {
		mergedTemplate = append(mergedTemplate, extend.GetText())
	}

	//haveImplements := false
	if i.IMPLEMENTS() != nil {
		//haveImplements = true
		mergedTemplate = append(mergedTemplate, i.TypeList(0).GetText())
	}

	//if i.PERMITS() != nil {
	//	idx := 1
	//	if !haveImplements {
	//		idx = 0
	//	}
	//	log.Infof("class: %v java17 permits: %v", className, i.TypeList(idx).GetText())
	//}

	for _, parentClass := range mergedTemplate {
		if parent := y.GetClassBluePrint(parentClass); parent != nil {
			class.AddParentClass(parent)
		} else {
			class.AddParentClass(y.CreateClassBluePrint(parentClass))
		}
	}
	y.VisitClassBody(i.ClassBody(), class)

}

func (y *builder) VisitClassBody(raw javaparser.IClassBodyContext, class *ssa.ClassBluePrint) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassBodyContext)
	if i == nil {
		return nil
	}

	builders := make([]func(), len(i.AllClassBodyDeclaration()))
	for i, ret := range i.AllClassBodyDeclaration() {
		builders[i] = y.VisitClassBodyDeclaration(ret, class)
	}
	for _, build := range builders {
		build()
	}
	return nil
}
func (y *builder) VisitModifier(raw javaparser.IModifierContext) ssa.ClassModifier {
	var m ssa.ClassModifier
	if y == nil || raw == nil {
		return m
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ModifierContext)
	if i == nil {
		return m
	}

	if i.ClassOrInterfaceModifier() == nil {
		return m
	} else {
		return y.VisitClassOrInterfaceModifier(i.ClassOrInterfaceModifier())
	}

}

func (y *builder) VisitClassOrInterfaceModifier(raw javaparser.IClassOrInterfaceModifierContext) ssa.ClassModifier {
	var m ssa.ClassModifier
	if y == nil || raw == nil {
		return m
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassOrInterfaceModifierContext)
	if i == nil {
		return m
	}

	if i.PUBLIC() != nil {
		return ssa.Public
	} else if i.PROTECTED() != nil {
		return ssa.Protected
	} else if i.PRIVATE() != nil {
		return ssa.Private
	} else if i.STATIC() != nil {
		return ssa.Static
	} else if i.ABSTRACT() != nil {
		return ssa.Abstract
	} else if i.FINAL() != nil {
		return ssa.Final
	} else {
		return ssa.NoneModifier
	}
}

func (y *builder) VisitFormalParameters(raw javaparser.IFormalParametersContext) {
	if y == nil || raw == nil {
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

func (y *builder) VisitMemberDeclaration(raw javaparser.IMemberDeclarationContext, class *ssa.ClassBluePrint, isStatic bool) func() {
	if y == nil || raw == nil {
		return func() {}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.MemberDeclarationContext)
	if i == nil {
		return func() {}
	}

	if ret := i.RecordDeclaration(); ret != nil {
		log.Infof("todo: java17: %v", ret.GetText())
	} else if ret := i.MethodDeclaration(); ret != nil {
		return y.VisitMethodDeclaration(ret, class, isStatic)
	} else if ret := i.GenericMethodDeclaration(); ret != nil {
	} else if ret := i.FieldDeclaration(); ret != nil {
		// 声明成员变量
		setMember := class.AddNormalMember
		if isStatic {
			setMember = class.AddStaticMember
		}
		field := ret.(*javaparser.FieldDeclarationContext)

		if field.TypeType() == nil {
			y.VisitTypeType(field.TypeType())
		}

		variableDeclarators := field.VariableDeclarators().(*javaparser.VariableDeclaratorsContext).AllVariableDeclarator()
		for _, variableDeclarator := range variableDeclarators {
			v := variableDeclarator.(*javaparser.VariableDeclaratorContext)
			name, value := y.VisitVariableDeclarator(v)
			setMember(name, value)
		}

	} else if ret := i.ConstructorDeclaration(); ret != nil {
		//声明构造函数
		y.VisitConstructorDeclaration(ret, class)

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

	return func() {}
}

func (y *builder) VisitTypeType(raw javaparser.ITypeTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.TypeTypeContext)
	if i == nil {
		return nil
	}
	// todo annotation
	var t ssa.Type
	if ret := i.ClassOrInterfaceType(); ret != nil {
		t = y.VisitClassOrInterfaceType(ret)
	} else {
		t = y.VisitPrimitiveType(i.PrimitiveType())
	}

	return t
}

func (y *builder) VisitClassOrInterfaceType(raw javaparser.IClassOrInterfaceTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	// todo 类和接口的类型声明
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ClassOrInterfaceTypeContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitPrimitiveType(raw javaparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}
	switch i.GetText() {
	case "boolean":
		return ssa.GetBooleanType()
	case "char", "short", "int", "long", "float", "double":
		return ssa.GetNumberType()
	case "byte":
		return ssa.GetBytesType()
	default:
		return ssa.GetAnyType()
	}
}

func (y *builder) VisitEnumDeclaration(raw javaparser.IEnumDeclarationContext, class *ssa.ClassBluePrint) interface{} {
	if y == nil || raw == nil {
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
		class = y.CreateClassBluePrint(enumName)
	}

	if i.IMPLEMENTS() != nil {
		mergedTemplate = append(mergedTemplate, i.TypeList().GetText())
	}

	for _, parentClass := range mergedTemplate {
		if parent := y.GetClassBluePrint(parentClass); parent != nil {
			class.AddParentClass(parent)
		} else {
			class.AddParentClass(y.CreateClassBluePrint(parentClass))
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

func (y *builder) VisitEnumConstants(raw javaparser.IEnumConstantsContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
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
	setMember := class.AddNormalMember
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

func (y *builder) VisitEnumConstant(raw javaparser.IEnumConstantContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
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

	setMember := class.AddStaticMember

	name := i.Identifier().GetText()
	variable := y.CreateVariable(name)
	_ = variable
	setMember(name, y.EmitConstInstAny())
	return
}

func (y *builder) VisitEnumBodyDeclarations(raw javaparser.IEnumBodyDeclarationsContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
		return
	}
	i, _ := raw.(*javaparser.EnumBodyDeclarationsContext)
	if i == nil {
		return
	}

	builders := make([]func(), len(i.AllClassBodyDeclaration()))
	for i, ret := range i.AllClassBodyDeclaration() {
		builders[i] = y.VisitClassBodyDeclaration(ret, class)
	}
	for _, build := range builders {
		build()
	}
}

func (y *builder) VisitClassBodyDeclaration(raw javaparser.IClassBodyDeclarationContext, class *ssa.ClassBluePrint) func() {
	if y == nil || raw == nil {
		return func() {}
	}

	i, _ := raw.(*javaparser.ClassBodyDeclarationContext)
	if i == nil {
		return func() {}
	}

	if ret := i.Block(); ret != nil {
		y.VisitBlock(i.Block())
	} else if ret := i.MemberDeclaration(); ret != nil {
		var modifiers = make(map[ssa.ClassModifier]struct{})
		for _, modifier := range i.AllModifier() {
			m := y.VisitModifier(modifier)
			modifiers[m] = struct{}{}
		}
		isStatic := false
		if _, ok := modifiers[ssa.Static]; ok {
			isStatic = true
		}
		if class != nil {
			return y.VisitMemberDeclaration(ret, class, isStatic)
		}

	}
	return func() {}
}
func (y *builder) VisitInterfaceDeclaration(raw javaparser.IInterfaceDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.InterfaceDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitAnnotationTypeDeclaration(raw javaparser.IAnnotationTypeDeclarationContext) interface{} {
	if y == nil || raw == nil {
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

func (y *builder) VisitRecordDeclaration(raw javaparser.IRecordDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.RecordDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitMethodDeclaration(raw javaparser.IMethodDeclarationContext, class *ssa.ClassBluePrint, isStatic bool) func() {
	if y == nil || raw == nil {
		return func() {}
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.MethodDeclarationContext)
	if i == nil {
		return func() {}
	}

	key := i.Identifier().GetText()
	pkgPath := y.GetCurrentPackagePath()
	pkgName := strings.Join(pkgPath, "_")
	funcName := fmt.Sprintf("%s_%s_%s", pkgName, class.Name, key)

	if isStatic {
		newFunction := y.NewFunc(funcName)

		build := func() {
			y.FunctionBuilder = y.PushFunction(newFunction)
			y.MarkedThisClassBlueprint = class
			y.VisitFormalParameters(i.FormalParameters())
			y.VisitMethodBody(i.MethodBody())
			y.SetType(y.VisitTypeTypeOrVoid(i.TypeTypeOrVoid()))
			y.Finish()
			y.FunctionBuilder = y.PopFunction()
			y.AddToPackage(funcName)
		}

		y.AssignClassConst(class.Name, key, newFunction)
		if i.THROWS() != nil {
			if qualifiedNameList := i.QualifiedNameList(); qualifiedNameList != nil {
				y.VisitQualifiedNameList(qualifiedNameList)
			}
		}
		return build
	}
	newFunction := y.NewFunc(funcName)
	build := func() {
		y.FunctionBuilder = y.PushFunction(newFunction)
		y.MarkedThisClassBlueprint = class
		this := y.NewParam("this")
		_ = this
		y.VisitFormalParameters(i.FormalParameters())
		y.VisitMethodBody(i.MethodBody())
		y.Finish()
		y.FunctionBuilder = y.PopFunction()
		y.AddToPackage(funcName)
	}

	if i.THROWS() != nil {
		if qualifiedNameList := i.QualifiedNameList(); qualifiedNameList != nil {
			y.VisitQualifiedNameList(qualifiedNameList)
		}

	}
	class.AddMethod(key, newFunction)
	return build
}

func (y *builder) VisitMethodBody(raw javaparser.IMethodBodyContext) {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
		return ssa.GetAnyType()
	}

}

func (y *builder) VisitFormalParameterList(raw javaparser.IFormalParameterListContext) {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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

func (y *builder) VisitFormalParameter(raw javaparser.IFormalParameterContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.FormalParameterContext)
	if i == nil {
		return
	}
	for _, modifier := range i.AllVariableModifier() {
		y.VisitVariableModifier(modifier)
	}
	typeType := y.VisitTypeType(i.TypeType())
	formalParams := y.VisitVariableDeclaratorId(i.VariableDeclaratorId())
	param := y.NewParam(formalParams)
	if typeType != nil {
		param.SetType(typeType)
	}

}

func (y *builder) VisitVariableDeclaratorId(raw javaparser.IVariableDeclaratorIdContext) string {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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

func (y *builder) VisitVariableModifier(raw javaparser.IVariableModifierContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.VariableModifierContext)
	if i == nil {
		return
	}
}

func (y *builder) VisitQualifiedNameList(raw javaparser.IQualifiedNameListContext) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.QualifiedNameListContext)
	if i == nil {
		return
	}

}

func (y *builder) VisitConstructorDeclaration(raw javaparser.IConstructorDeclarationContext, class *ssa.ClassBluePrint) {
	if y == nil || raw == nil {
		return
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ConstructorDeclarationContext)
	if i == nil {
		return
	}

	key := i.Identifier().GetText()
	pkgPath := y.GetCurrentPackagePath()
	pkgName := strings.Join(pkgPath, "_")
	funcName := fmt.Sprintf("%s_%s_%s", pkgName, class.Name, key)

	createFunction := func() *ssa.Function {
		newFunction := y.NewFunc(funcName)
		y.FunctionBuilder = y.PushFunction(newFunction)
		{
			this := y.NewParam("this")
			_ = this
			y.VisitFormalParameters(i.FormalParameters())
			y.VisitBlock(i.Block())
			y.Finish()
			y.AddToPackage(funcName)
		}
		y.FunctionBuilder = y.PopFunction()
		return newFunction
	}

	if i.THROWS() != nil {
		y.VisitQualifiedNameList(i.QualifiedNameList())
	}
	newFunction := createFunction()
	class.Constructor = newFunction

}
