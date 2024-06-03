package java2ssa

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func (y *builder) VisitAllImport(i *javaparser.CompilationUnitContext) {
	for _, pkgImport := range i.AllImportDeclaration() {
		pkgNames, static, all := y.VisitImportDeclaration(pkgImport)
		_, _, _ = pkgNames, static, all

		var pkg *ssa.Package
		var className string
		// found package
		for i := len(pkgNames) - 1; i > 0; i-- {
			className = strings.Join(pkgNames[i:], ".")
			if pkg = y.GetPackage(strings.Join(pkgNames[:i], ".")); pkg != nil {
				break
			}
			if pkg = y.BuildPackage(pkgNames[:i]); pkg != nil {
				break
			}
		}
		if pkg == nil {
			log.Errorf("BUG: Import package %v but not found", pkgNames)
			return
		}

		// get class
		if all {
			for _, class := range pkg.ClassBluePrint {
				y.SetClassBluePrint(class.Name, class)
			}
		} else if class := pkg.GetClassBluePrint(className); class != nil {
			y.SetClassBluePrint(className, class)
		} else {
			log.Warnf("BUG: Import  class %s but nod found in package %v", className, pkg.Name)
		}
	}
}

func (y *builder) BuildPackage(pkgNames []string) *ssa.Package {
	if y == nil {
		return nil
	}
	prog := y.GetProgram()
	if prog == nil {
		return nil
	}
	loader := prog.Loader

	pkgPath := strings.Join(pkgNames, "/")
	_ = pkgPath

	ch, err := loader.LoadDirectoryPackage(pkgPath, true)
	if err != nil {
		return nil
	}
	for fd := range ch {
		y.LoadPackageByPath(prog, loader, fd.FileName, fd.File)
		fd.File.Close()
	}

	pkgName := strings.Join(pkgNames, ".")
	return y.GetPackage(pkgName)
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
