//go:build !no_language
// +build !no_language

package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
)

func (y *singleFileBuilder) VisitCompilationUnit(raw javaparser.ICompilationUnitContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
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
		lib, _ := prog.GetLibrary(pkgName)
		//if skip {
		// log.Infof("package %v skip, file %v", pkgName, prog.GetCurrentEditor().GetFilename())
		//return nil
		//}
		// log.Infof("package %v parse, file %v", pkgName, prog.GetCurrentEditor().GetFilename())
		if lib == nil {
			lib = prog.NewLibrary(pkgName, pkgPath)
		}
		lib.PushEditor(prog.GetApplication().GetCurrentEditor())

		builder := lib.GetAndCreateFunctionBuilder(pkgName, string(ssa.MainFunctionName))
		if builder != nil {
			builder.SetEditor(prog.GetApplication().GetCurrentEditor())
			builder.SetBuildSupport(y.FunctionBuilder)
			currentBuilder := y.FunctionBuilder
			y.FunctionBuilder = builder
			defer func() {
				y.FunctionBuilder = currentBuilder
			}()
		}
	}

	/*
		pre handler 情况下只记录import fullType记录
	*/
	y.VisitAllImport(i)
	if y.PreHandler() {
		for _, declarationContext := range i.AllTypeDeclaration() {
			y.VisitTypeDeclaration(declarationContext)
		}
	}
	y.GetProgram().VisitAst(i)

	return nil
}

func (y *singleFileBuilder) VisitImportDeclaration(raw javaparser.IImportDeclarationContext) (packagePath []string, static bool, importAll bool) {
	if y == nil || raw == nil || y.IsStop() {
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
	res := y.VisitQualifiedName(i.QualifiedName())
	if importAll {
		res = append(res, i.MUL().GetText())
	}
	return res, static, importAll
}

func (y *singleFileBuilder) VisitPackageDeclaration(raw javaparser.IPackageDeclarationContext) []string {
	if y == nil || raw == nil || y.IsStop() {
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
func (b *singleFileBuilder) SwitchProg(functionBuilder *ssa.FunctionBuilder, editor *memedit.MemEditor) func() {
	//log.Infof("lazyBuilder current File: %s", currentFile)
	currentfb := b.FunctionBuilder
	currenteditor := b.FunctionBuilder.GetEditor()
	b.FunctionBuilder = functionBuilder
	b.FunctionBuilder.SetEditor(editor)
	return func() {
		b.FunctionBuilder = currentfb
		b.FunctionBuilder.SetEditor(currenteditor)
	}
}

func (y *singleFileBuilder) VisitPackageName(raw javaparser.IPackageNameContext) []string {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	i, _ := raw.(*javaparser.PackageNameContext)
	if name := i.QualifiedName(); name != nil {
		return y.VisitQualifiedName(name)
	} else {
		// TODO: handler `package ${package}.action` this code use with maven
		return nil
	}
}

func (y *singleFileBuilder) VisitQualifiedName(raw javaparser.IQualifiedNameContext) []string {
	if y == nil || raw == nil || y.IsStop() {
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
