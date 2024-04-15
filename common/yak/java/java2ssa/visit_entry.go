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
			y.FunctionBuilder = builder
		}

	}

	for _, pkgImport := range i.AllImportDeclaration() {
		paths, static, all := y.VisitImportDeclaration(pkgImport)
		log.Infof("import %v (static: %v) (all: %v)", paths, static, all)
	}

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
	return y.VisitPackageQualifiedName(i.QualifiedName()), static, importAll
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

	packagePath := y.VisitPackageQualifiedName(i.QualifiedName())
	return packagePath
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
