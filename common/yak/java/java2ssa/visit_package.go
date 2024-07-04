package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func (y *builder) VisitAllImport(i *javaparser.CompilationUnitContext) {
	for _, pkgImport := range i.AllImportDeclaration() {
		pkgNames, static, all := y.VisitImportDeclaration(pkgImport)
		_, _, _ = pkgNames, static, all

		var prog *ssa.Program
		var className string
		// found package
		for i := len(pkgNames) - 1; i > 0; i-- {
			className = strings.Join(pkgNames[i:], ".")
			if p, err := ssa.GetProgram(strings.Join(pkgNames[:i], "."), ssa.Library); err == nil {
				prog = p
				break
			}
			if prog = y.BuildPackage(pkgNames[:i]); prog != nil {
				break
			}
		}
		if prog == nil {
			log.Warnf("Dependencies Missed: Import package %v but not found", pkgNames)
			return
		}

		// get class
		if all {
			for _, class := range prog.ClassBluePrint {
				y.SetClassBluePrint(class.Name, class)
			}
		} else if class := prog.GetClassBluePrint(className); class != nil {
			y.SetClassBluePrint(className, class)
		} else {
			log.Warnf("BUG: Import  class %s but not found in package %v", className, prog.Name)
		}
	}
}

func (y *builder) BuildPackage(pkgNames []string) *ssa.Program {
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
		raw, err := loader.GetFilesysFileSystem().ReadFile(fd.FileName)
		if err != nil {
			log.Errorf("Build with file loader failed: %s", err)
			continue
		}
		y.LoadPackageByPath(prog, loader, fd.FileName, memedit.NewMemEditor(string(raw)))
	}

	pkgName := strings.Join(pkgNames, ".")
	if p, err := ssa.GetProgram(pkgName, ssa.Library); err == nil {
		return p
	}
	return nil
}

func (y *builder) LoadPackageByPath(prog *ssa.Program, loader *ssautil.PackageFileLoader, fileName string, data *memedit.MemEditor) {
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
