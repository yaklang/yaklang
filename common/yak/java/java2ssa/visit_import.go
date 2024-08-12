package java2ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitAllImport(i *javaparser.CompilationUnitContext) {
	start := time.Now()
	defer func() {
		deltaPackageCostFrom(start)
	}()

	for _, pkgImport := range i.AllImportDeclaration() {
		pkgNames, static, all := y.VisitImportDeclaration(pkgImport)
		if len(pkgNames) > 0 {
			// 用于遍历所有import的类，并添加到fullTypeNameMap中
			if all {
				y.allImportPkgSlice = append(y.allImportPkgSlice, pkgNames)
			} else {
				y.fullTypeNameMap[pkgNames[len(pkgNames)-1]] = pkgNames
			}
		}

		_, _, _ = pkgNames, static, all

		var prog *ssa.Program
		var className string
		// found package
		for i := len(pkgNames) - 1; i > 0; i-- {
			className = strings.Join(pkgNames[i:], ".")
			if lib, _ := y.GetProgram().GetLibrary(strings.Join(pkgNames[:i], ".")); lib != nil {
				prog = lib
				break
			}
			if p, err := y.BuildDirectoryPackage(pkgNames[:i], true); err == nil {
				prog = p
				break
			} else {
				log.Infof("Dependencies Missed: Import package not found(%v)", err)
			}
		}
		if prog == nil {
			log.Warnf("Dependencies Missed: Import package %v but not found", pkgNames)
			continue
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
