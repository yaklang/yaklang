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
	var start time.Time
	if javaCompileProfileOn() {
		start = time.Now()
		defer func() {
			deltaPackageCostFrom(start)
		}()
	}

	hasImportType := func(lib *ssa.Program, name string) bool {
		if lib == nil || name == "" {
			return false
		}
		candidates := []string{name}
		if strings.Contains(name, ".") {
			candidates = append(candidates, strings.ReplaceAll(name, ".", INNER_CLASS_SPLIT))
		}
		for _, candidate := range candidates {
			if _, ok := lib.GetExportType(candidate); ok {
				return true
			}
			if lib.GetBluePrint(candidate) != nil {
				return true
			}
		}
		return false
	}

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

		// A package occupied by another prehandle worker must not be compiled
		// here: BuildDirectoryPackage enters that library's main builder.
		// Names are already recorded. The deferred task links after the owner
		// releases the slot. A package nobody is compiling is still compiled
		// now, including a dependency that belongs to a later batch.
		deferForeignPackage := !resolveStaticTypes && y.GetProgram() != nil && y.GetProgram().ConcurrentUnitBuildEnabled()
		ownerBusy := false

		var prog *ssa.Program
		var className string
		// found package
		if static {
			var (
				lastErr           error
				valName           string
				fallbackProg      *ssa.Program
				fallbackClassName string
			)
			classEnd := len(pkgNames) - 1
			if !all && len(pkgNames) > 0 {
				valName = pkgNames[len(pkgNames)-1]
			}
			tryLibrary := func(lib *ssa.Program, candidate string, allowFallback bool) bool {
				if lib == nil {
					return false
				}
				if hasImportType(lib, candidate) {
					prog = lib
					className = candidate
					return true
				}
				if allowFallback && fallbackProg == nil {
					fallbackProg = lib
					fallbackClassName = candidate
				}
				return false
			}
			unitLibrary := false
			for idx := classEnd - 1; idx > 0; idx-- {
				pkgName := strings.Join(pkgNames[:idx], ".")
				candidateClassName := strings.Join(pkgNames[idx:classEnd], ".")
				if deferForeignPackage && y.packageIsCompileUnit(pkgName) {
					unitLibrary = true
					if lib, _ := y.GetProgram().GetLibrary(pkgName); tryLibrary(lib, candidateClassName, false) {
						break
					}
					ownerBusy = true
					break
				}
				// The post-skeleton link runs after the owner finished. The unit
				// library already exists; compiling the directory again duplicates
				// the class. hasFile is not existence.
				if resolveStaticTypes {
					if lib, _ := y.GetProgram().GetLibrary(pkgName); lib != nil {
						unitLibrary = true
						if tryLibrary(lib, candidateClassName, false) {
							break
						}
						continue
					}
				}
				if lib, ok := y.GetProgram().GetLibrary(pkgName); ok && tryLibrary(lib, candidateClassName, false) {
					break
				}
				if deferForeignPackage && y.directoryPackageBusy(pkgName) {
					ownerBusy = true
					continue
				}
				if p, err := y.BuildDirectoryPackage(pkgNames[:idx], true); err == nil {
					if tryLibrary(p, candidateClassName, false) {
						break
					}
				} else {
					lastErr = err
				}
			}
			if prog == nil && !ownerBusy && !unitLibrary {
				for idx := classEnd - 1; idx > 0; idx-- {
					pkgName := strings.Join(pkgNames[:idx], ".")
					candidateClassName := strings.Join(pkgNames[idx:classEnd], ".")
					if lib, err := y.GetProgram().GetOrCreateLibrary(pkgName); err == nil && tryLibrary(lib, candidateClassName, true) {
						break
					}
				}
			}
			if prog == nil && fallbackProg != nil {
				prog = fallbackProg
				className = fallbackClassName
			}
			if prog == nil && lastErr != nil {
				log.Infof("Dependencies Missed: Import package not found(%v)", lastErr)
			}
			if prog != nil && resolveStaticTypes {
				if all {
					_ = y.GetProgram().ImportTypeStaticAll(prog, className)
				} else {
					_ = y.GetProgram().ImportTypeStaticMemberFromLib(prog, className, valName)
				}
			}
		} else {
			var lastErr error
			unitLibrary := false
			for idx := len(pkgNames) - 1; idx > 0; idx-- {
				className = strings.Join(pkgNames[idx:], ".")
				pkgName := strings.Join(pkgNames[:idx], ".")
				// The scheduler owns every project unit, including units whose
				// library has not been created yet. Recursively compiling one here
				// would declare its methods again when its scheduled task starts.
				// Link missing project types after all skeletons are available;
				// external imports still use the directory/virtual fallback below.
				if deferForeignPackage && y.packageIsCompileUnit(pkgName) {
					unitLibrary = true
					if lib, _ := y.GetProgram().GetLibrary(pkgName); lib != nil && (all || hasImportType(lib, className)) {
						prog = lib
					} else {
						ownerBusy = true
					}
					break
				}
				// Post-skeleton: do not compile a package the unit scheduler
				// already owns. A second build shows up as an extra typeName.
				if resolveStaticTypes {
					if lib, _ := y.GetProgram().GetLibrary(pkgName); lib != nil {
						unitLibrary = true
						if all || hasImportType(lib, className) {
							prog = lib
							break
						}
						continue
					}
				}
				// hasFile, not mere existence. An empty unit library must not
				// hide the directory compile that actually builds the class.
				// A file of this package can already be registered while a later
				// class in the same unit is still undeclared. Importing that name
				// now makes fakeImportType plant an empty blueprint in the owner's
				// map. Leave it for the post-skeleton binding pass.
				if lib, ok := y.GetProgram().GetLibrary(pkgName); ok && lib != nil {
					if deferForeignPackage && !hasImportType(lib, className) {
						unitLibrary = true
						ownerBusy = true
						break
					}
					prog = lib
					break
				}
				if deferForeignPackage && y.directoryPackageBusy(pkgName) {
					ownerBusy = true
					continue
				}
				if p, err := y.BuildDirectoryPackage(pkgNames[:idx], true); err == nil {
					prog = p
					break
				} else {
					lastErr = err
				}
			}
			if prog == nil && !ownerBusy && !unitLibrary {
				for idx := len(pkgNames) - 1; idx > 0; idx-- {
					className = strings.Join(pkgNames[idx:], ".")
					pkgName := strings.Join(pkgNames[:idx], ".")
					if lib, err := y.GetProgram().GetOrCreateLibrary(pkgName); err == nil && lib != nil {
						prog = lib
						break
					}
				}
			}
			if prog == nil && lastErr != nil {
				log.Infof("Dependencies Missed: Import package not found(%v)", lastErr)
			}
		}
		if prog == nil {
			if !ownerBusy {
				log.Warnf("Dependencies Missed: Import package %v but not found", pkgNames)
			}
			continue
		}
		prog.PushEditor(y.GetEditor())
		// get class
		if all {
			_ = y.GetProgram().ImportAll(prog)
			// Project declarations use ExportType; ImportAll also supports the
			// legacy external-type map, which does not contain these skeletons.
			if prog.ExportType != nil {
				for name := range prog.ExportType.GetAll() {
					_ = y.GetProgram().ImportTypeFromLib(prog, name, pkgImport.token)
				}
			}
		} else {
			_ = y.GetProgram().ImportTypeFromLib(prog, className, pkgImport.token)
		}
		prog.PopEditor(false)
	}
}

func (y *singleFileBuilder) packageIsCompileUnit(pkgName string) bool {
	if y == nil || pkgName == "" || y.GetProgram() == nil {
		return false
	}
	app := y.GetProgram().GetApplication()
	if app == nil {
		app = y.GetProgram()
	}
	want := "java:" + pkgName
	for _, unit := range app.CompileUnits {
		if unit != nil && unit.Key == want {
			return true
		}
	}
	return false
}

func (y *singleFileBuilder) directoryPackageBusy(pkgName string) bool {
	if y == nil || pkgName == "" || y.GetProgram() == nil {
		return false
	}
	app := y.GetProgram().GetApplication()
	if app == nil {
		app = y.GetProgram()
	}
	return app.DirectoryPackageInProgress(pkgName)
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

// registerPostSkeletonImportTask schedules import linking for pass2, after the
// parallel skeleton barrier, without retaining the whole file AST.
func (y *singleFileBuilder) registerPostSkeletonImportTask(i *javaparser.CompilationUnitContext) {
	if y == nil || i == nil {
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
	fileEditor := y.GetEditor()
	if fileEditor == nil {
		fileEditor = app.GetCurrentEditor()
	}
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
	prog.RegisterDeferredBuild(ssa.DeferredBuildKindBinding, taskKey, func() {
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
