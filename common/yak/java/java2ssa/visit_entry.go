package java2ssa

import (
	"strings"

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
		pkgName := strings.Join(pkgPath, ".")
		prog := y.GetProgram()
		lib, skip := prog.GetLibrary(pkgName)
		if skip {
			return nil
		}
		if lib == nil {
			lib = prog.NewLibrary(pkgName, pkgPath)
		}
		lib.PushEditor(prog.GetCurrentEditor())

		builder := lib.GetAndCreateFunctionBuilder(pkgName, "init")
		if builder != nil {
			builder.SetBuildSupport(y.FunctionBuilder)
			currentBuilder := y.FunctionBuilder
			y.FunctionBuilder = builder
			defer func() {
				y.FunctionBuilder = currentBuilder
			}()
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

	selfPkgPath := append(packagePath, "*")
	y.selfPkgPath = selfPkgPath
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
