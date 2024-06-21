package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

func (y *builder) VisitCompilationUnit(raw javaparser.ICompilationUnitContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*javaparser.CompilationUnitContext)
	if i == nil {
		return nil
	}

	if ret := i.PackageDeclaration(); ret != nil {
		pkgPath := y.VisitPackageDeclaration(ret)
		builder := y.AddCurrentPackagePath(pkgPath)
		if builder != nil {
			builder.SupportClass = true
			y.FunctionBuilder = builder
		}
	}
	y.VisitAllImport(i)
	for _, inst := range i.AllTypeDeclaration() {
		y.VisitTypeDeclaration(inst)
	}

	if ret := i.ModuleDeclaration(); ret != nil {
		y.VisitModuleDeclaration(ret)
	}

	return nil
}

func (y *builder) VisitImportDeclaration(raw javaparser.IImportDeclarationContext) (packagePath []string, static bool, importAll bool) {
	if y == nil || raw == nil {
		return nil, false, false
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ImportDeclarationContext)
	if i == nil {
		return nil, false, false
	}

	static = i.STATIC() != nil
	importAll = i.MUL() != nil
	res := y.VisitPackageQualifiedName(i.QualifiedName())
	if importAll {
		res = append(res, i.MUL().GetText())
	}
	return res, static, importAll
}

func (y *builder) VisitPackageDeclaration(raw javaparser.IPackageDeclarationContext) []string {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.PackageDeclarationContext)
	if i == nil {
		return nil
	}

	for _, pkgAnnotation := range i.AllAnnotation() {
		_ = pkgAnnotation
		log.Warnf("package annotation is not finished yet: %v", pkgAnnotation.GetText())
	}

	packagePath := y.VisitPackageName(i.PackageName())
	return packagePath
}

func (y *builder) VisitPackageName(raw javaparser.IPackageNameContext) []string {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*javaparser.PackageNameContext)
	if name := i.QualifiedName(); name != nil {
		return y.VisitPackageQualifiedName(name)
	} else {
		// TODO: handler `package ${package}.action` this code use with maven
		return nil
	}
}

func (y *builder) VisitPackageQualifiedName(raw javaparser.IQualifiedNameContext) []string {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.QualifiedNameContext)
	if i == nil {
		return nil
	}

	ret := i.AllIdentifier()
	result := make([]string, len(ret))
	for idx := 0; idx < len(ret); idx++ {
		result[idx] = i.Identifier(idx).GetText()
	}
	return result
}
