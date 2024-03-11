package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeDeclaration(raw javaparser.ITypeDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.TypeDeclarationContext)
	if i == nil {
		return nil
	}

	var modifier []string
	for _, mod := range i.AllClassOrInterfaceModifier() {
		modifier = append(modifier, mod.GetText())
	}

	if ret := i.ClassDeclaration(); ret != nil {
		return y.VisitClassDeclaration(ret)
	} else if ret := i.EnumDeclaration(); ret != nil {
		return y.VisitEnumDeclaration(ret)
	} else if ret := i.InterfaceDeclaration(); ret != nil {
		return y.VisitInterfaceDeclaration(ret)
	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {
		return y.VisitAnnotationTypeDeclaration(ret)
	} else if ret := i.RecordDeclaration(); ret != nil {
		return y.VisitRecordDeclaration(ret)
	}

	log.Errorf("visit type decl failed: %s", "unknown type")
	return nil
}

func (y *builder) VisitClassDeclaration(raw javaparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ClassDeclarationContext)
	if i == nil {
		return nil
	}

	className := i.Identifier().GetText()
	log.Infof("building class: %v", className)

	// Generic Type
	if ret := i.TypeParameters(); ret != nil {
		log.Infof("class: %v 's (generic type) type is %v, ignore for ssa building", className, ret.GetText())
	}

	// Extend Type
	if extend := i.TypeType(); extend != nil {
		log.Infof("class: %v extend: %s", className, extend.GetText())
		y.VisitTypeType(extend)
	}

	haveImplements := false
	if i.IMPLEMENTS() != nil {
		haveImplements = true
		log.Infof("class: %v implemented %v is ignored", className, i.TypeList(0).GetText())
	}

	if i.PERMITS() != nil {
		idx := 1
		if !haveImplements {
			idx = 0
		}
		log.Infof("class: %v java17 permits: %v", className, i.TypeList(idx).GetText())
	}

	y.VisitClassBody(i.ClassBody())

	return nil
}

func (y *builder) VisitClassBody(raw javaparser.IClassBodyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ClassBodyContext)
	if i == nil {
		return nil
	}

	ir := y
	allMembers := i.AllClassBodyDeclaration()
	pb := ir.EmitNewClassBluePrint(len(allMembers))
	for _, decl := range allMembers {
		instance := decl.(*javaparser.ClassBodyDeclarationContext)
		if ret := instance.Block(); ret != nil {
			isStatic := instance.STATIC() != nil
			log.Infof("handle class static code: %v", isStatic)
			y.VisitBlock(instance.Block())
		} else if ret := instance.MemberDeclaration(); ret != nil {
			y.VisitMemberDeclaration(pb, ret)
		}
	}

	return nil
}

func (y *builder) VisitFormalParameters(raw javaparser.IFormalParametersContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.FormalParametersContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitMemberDeclaration(klass *ssa.ClassBluePrint, raw javaparser.IMemberDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.MemberDeclarationContext)
	if i == nil {
		return nil
	}

	ir := y
	if ret := i.RecordDeclaration(); ret != nil {
		log.Infof("todo: java17: %v", ret.GetText())
	} else if ret := i.MethodDeclaration(); ret != nil {
		log.Infof("method declearation: %v", ret.GetText())
		// create function
		method := ret.(*javaparser.MethodDeclarationContext)
		funcName := method.Identifier().GetText()
		ir.NewFunc(funcName)
		// TODO : handle OOP
		methodBody := method.MethodBody().(*javaparser.MethodBodyContext)
		ir.VisitBlock(methodBody.Block())
	} else if ret := i.GenericMethodDeclaration(); ret != nil {
	} else if ret := i.FieldDeclaration(); ret != nil {
		// 声明成员变量
		field := ret.(*javaparser.FieldDeclarationContext)
		variables := field.VariableDeclarators().(*javaparser.VariableDeclaratorsContext).AllVariableDeclarator()
		for _, variable := range variables {
			y.CreateLocalVariable(variable.GetText())
			log.Infof("create member declaration%v", variable.GetText())
		}

	} else if ret := i.ConstructorDeclaration(); ret != nil {

	} else if ret := i.GenericConstructorDeclaration(); ret != nil {

	} else if ret := i.InterfaceDeclaration(); ret != nil {

	} else if ret := i.AnnotationTypeDeclaration(); ret != nil {

	} else if ret := i.ClassDeclaration(); ret != nil {

	} else if ret := i.EnumDeclaration(); ret != nil {

	} else {
		log.Errorf("no member declaration found: %v", i.GetText())
		return nil
	}

	return nil
}

func (y *builder) VisitTypeType(raw javaparser.ITypeTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.TypeTypeContext)
	if i == nil {
		return nil
	}

	if ret := i.ClassOrInterfaceType(); ret != nil {
		y.VisitClassOrInterfaceType(ret)
	} else {
		y.VisitPrimitiveType(i.PrimitiveType())
	}

	return nil
}

func (y *builder) VisitClassOrInterfaceType(raw javaparser.IClassOrInterfaceTypeContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ClassOrInterfaceTypeContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitPrimitiveType(raw javaparser.IPrimitiveTypeContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitEnumDeclaration(raw javaparser.IEnumDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.EnumDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitInterfaceDeclaration(raw javaparser.IInterfaceDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

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

	i, _ := raw.(*javaparser.RecordDeclarationContext)
	if i == nil {
		return nil
	}

	return nil
}
