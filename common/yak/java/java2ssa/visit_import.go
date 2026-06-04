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
	y.visitImportDeclarations(i.AllImportDeclaration(), !y.PreHandler())
}

// visitImportDeclarations links imported packages/types. resolveStaticTypes is false
// during pass1 skeleton (same as legacy PreHandler) and true for the deferred pass2
// root task registered when SkeletonTopLevelEnabled.
func (y *singleFileBuilder) visitImportDeclarations(decls []javaparser.IImportDeclarationContext, resolveStaticTypes bool) {
	if y == nil || len(decls) == 0 || y.IsStop() {
		return
	}
	start := time.Now()
	defer func() {
		deltaPackageCostFrom(start)
	}()
	for _, pkgImport := range decls {
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
				if resolveStaticTypes {
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
					log.Warnf("Dependencies Missed: Import package not found(%v)", err)
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

// registerPostSkeletonImportTask schedules static import type linking for pass2
// without capturing the whole file AST in the shared pipeline's top-level closure.
func (y *singleFileBuilder) registerPostSkeletonImportTask(i *javaparser.CompilationUnitContext) {
	if y == nil || i == nil || !ssa.SkeletonTopLevelEnabled() {
		return
	}
	decls := i.AllImportDeclaration()
	if len(decls) == 0 {
		return
	}
	prog := y.GetProgram()
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	fileEditor := app.GetCurrentEditor()
	capturedDecls := append([]javaparser.IImportDeclarationContext(nil), decls...)
	capturedFullType := make(map[string][]string, len(y.fullTypeNameMap))
	for k, v := range y.fullTypeNameMap {
		capturedFullType[k] = append([]string(nil), v...)
	}
	capturedAllImport := append([][]string(nil), y.allImportPkgSlice...)
	capturedSelfPkg := append([]string(nil), y.selfPkgPath...)
	store := y.StoreFunctionBuilder()
	taskKey := "java-import"
	if fileEditor != nil {
		if u := fileEditor.GetUrl(); u != "" {
			taskKey = u
		}
	}
	prog.RegisterRootTask(ssa.RootBuildKindTopLevel, "java-static-import:"+taskKey, func() {
		if fileEditor != nil {
			app.PushEditor(fileEditor)
			defer app.PopEditor(true)
		}
		y2 := &singleFileBuilder{
			constMap:          make(map[string]ssa.Value),
			fullTypeNameMap:   capturedFullType,
			allImportPkgSlice: capturedAllImport,
			selfPkgPath:       capturedSelfPkg,
		}
		y2.initImport()
		y2.LoadBuilder(store)
		y2.visitImportDeclarations(capturedDecls, true)
	})
}
