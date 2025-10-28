//go:build !no_language
// +build !no_language

package java2ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) VisitAllImport(i *javaparser.CompilationUnitContext) {
	if y == nil || i == nil || y.IsStop() {
		return
	}
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
		if static {
			pkg := pkgNames[:len(pkgNames)-2]
			className = pkgNames[len(pkgNames)-2]
			valName := pkgNames[len(pkgNames)-1]
			if library, _ := y.GetProgram().GetLibrary(strings.Join(pkg, ".")); library != nil {
				prog = library
				if !y.PreHandler() {
					if all {
						_ = y.GetProgram().ImportTypeStaticAll(prog, className)
					} else {
						_ = y.GetProgram().ImportTypeStaticMemberFromLib(prog, className, valName)
					}
				}
			}
		} else {
			for i := len(pkgNames) - 1; i > 0; i-- {
				className = strings.Join(pkgNames[i:], ".")
				if lib, _ := y.GetProgram().GetOrCreateLibrary(strings.Join(pkgNames[:i], ".")); lib != nil {
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
		}
		if prog == nil {
			log.Warnf("Dependencies Missed: Import package %v but not found", pkgNames)
			continue
		}
		prog.PushEditor(y.GetEditor())
		// get class
		if all {
			_ = y.GetProgram().ImportAll(prog)
		} else {
			_ = y.GetProgram().ImportTypeFromLib(prog, className, pkgImport)
		}
		prog.PopEditor(false)
	}
}
