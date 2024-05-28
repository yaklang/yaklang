package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
	"io"
	"strings"
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

	prog := y.GetProgram()
	loader := prog.Loader
	fs := loader.GetFilesysFileSystem()
	for _, pkgImport := range i.AllImportDeclaration() {
		paths, static, all := y.VisitImportDeclaration(pkgImport)
		_, _, _ = paths, static, all
		var verbose string
		if !static {
			verbose = strings.Join(paths, ".")
		} else {
			verbose = strings.Join(append(paths, "*"), "")
		}
		log.Warnf("TBD: ImportDeclaration %v", verbose)
		if all {
			packetDir := strings.Join(paths, string([]rune{fs.GetSeparators()}))
			fdChan, err := loader.LoadDirectoryPackage(packetDir, true)
			if err != nil {
				log.Warnf("package loader handle package directory failed: %v")
				continue
			}
			if fdChan != nil {
				for fd := range fdChan {
					y.LoadPackageByPath(prog, loader, fd.FileName, fd.Data)
				}
			}
		} else {
			fileName := strings.Join(paths, string([]rune{fs.GetSeparators()}))
			if strings.HasSuffix(fileName, ".java") {
				fileName += ".java"
			}
			filePathName, data, err := loader.LoadFilePackage(fileName, true)
			if err != nil {
				log.Warnf("package laoder handle file package failed: %v", err)
				continue
			}
			y.LoadPackageByPath(prog, loader, filePathName, data)
		}
	}

	for _, inst := range i.AllTypeDeclaration() {
		y.VisitTypeDeclaration(inst)
	}

	if ret := i.ModuleDeclaration(); ret != nil {
		y.VisitModuleDeclaration(ret)
	}

	return nil
}

func (y *builder) LoadPackageByPath(prog *ssa.Program, loader *ssautil.PackageLoader, fileName string, data io.Reader) {
	originPath := loader.GetCurrentPath()
	defer func() {
		loader.SetCurrentPath(originPath)
	}()
	err := prog.Build(fileName, data, y.FunctionBuilder)
	if err != nil {
		log.Warnf("TBD: Build via LoadPackageByPath failed: %v", err)
		return
	}
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
