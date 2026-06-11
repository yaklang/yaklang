package java2ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type javaImportDecl struct {
	pkgNames []string
	static   bool
	all      bool
	token    ssa.CanStartStopToken
}

func (y *singleFileBuilder) VisitAllImport(i *javaparser.CompilationUnitContext) {
	if y == nil || i == nil || y.IsStop() {
		return
	}
	y.visitImportDeclarations(i.AllImportDeclaration(), !y.PreHandler())
}

// visitImportDeclarations links imported packages/types. resolveStaticTypes is false
// during pass1 skeleton (same as legacy PreHandler) and true for the deferred pass.
func (y *singleFileBuilder) visitImportDeclarations(decls []javaparser.IImportDeclarationContext, resolveStaticTypes bool) {
	if y == nil || len(decls) == 0 || y.IsStop() {
		return
	}
	imports := make([]javaImportDecl, 0, len(decls))
	for _, decl := range decls {
		if item, ok := newJavaImportDecl(decl); ok {
			imports = append(imports, item)
		}
	}
	y.visitCapturedImportDeclarations(imports, resolveStaticTypes)
}

func (y *singleFileBuilder) visitCapturedImportDeclarations(decls []javaImportDecl, resolveStaticTypes bool) {
	if y == nil || len(decls) == 0 || y.IsStop() {
		return
	}
	start := time.Now()
	defer func() {
		deltaPackageCostFrom(start)
	}()
	for _, pkgImport := range decls {
		pkgNames, static, all := pkgImport.pkgNames, pkgImport.static, pkgImport.all
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
			_ = y.GetProgram().ImportTypeFromLib(prog, className, pkgImport.token)
		}
		prog.PopEditor(false)
	}
}

func newJavaImportDecl(raw javaparser.IImportDeclarationContext) (javaImportDecl, bool) {
	i, _ := raw.(*javaparser.ImportDeclarationContext)
	if i == nil {
		return javaImportDecl{}, false
	}
	pkgNames := javaQualifiedNameParts(i.QualifiedName())
	importAll := i.MUL() != nil
	if importAll {
		pkgNames = append(pkgNames, i.MUL().GetText())
	}
	return javaImportDecl{
		pkgNames: pkgNames,
		static:   i.STATIC() != nil,
		all:      importAll,
		token:    ssa.NewTextRangeToken(i),
	}, true
}

func javaQualifiedNameParts(raw javaparser.IQualifiedNameContext) []string {
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
	capturedDecls := make([]javaImportDecl, 0, len(decls))
	for _, decl := range decls {
		if item, ok := newJavaImportDecl(decl); ok {
			capturedDecls = append(capturedDecls, item)
		}
	}
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
	prog.RegisterDeferredBuild(ssa.DeferredBuildKindHelper, "java-static-import:"+taskKey, func() {
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
		y2.visitCapturedImportDeclarations(capturedDecls, true)
	})
}
