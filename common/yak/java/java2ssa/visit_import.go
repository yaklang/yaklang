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

	for _, pkgImport := range i.AllImportDeclaration() {
		pkgNames, static, all := y.VisitImportDeclaration(pkgImport)
		if len(pkgNames) > 0 {
			if all {
				y.allImportPkgSlice = append(y.allImportPkgSlice, pkgNames)
			} else {
				y.fullTypeNameMap[pkgNames[len(pkgNames)-1]] = pkgNames
			}
		}

		var prog *ssa.Program
		var className string
		if static {
			var (
				lastErr           error
				valName           string
				fallbackProg      *ssa.Program
				fallbackClassName string
			)
			classEnd := len(pkgNames)
			if all {
				classEnd = len(pkgNames) - 1
			} else {
				classEnd = len(pkgNames) - 1
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
			for idx := classEnd - 1; idx > 0; idx-- {
				pkgName := strings.Join(pkgNames[:idx], ".")
				candidateClassName := strings.Join(pkgNames[idx:classEnd], ".")
				if lib, ok := y.GetProgram().GetLibrary(pkgName); ok && tryLibrary(lib, candidateClassName, false) {
					break
				}
				if p, err := y.BuildDirectoryPackage(pkgNames[:idx], true); err == nil {
					if tryLibrary(p, candidateClassName, false) {
						break
					}
				} else {
					lastErr = err
				}
			}
			if prog == nil {
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
			if prog != nil && !y.PreHandler() {
				if all {
					_ = y.GetProgram().ImportTypeStaticAll(prog, className)
				} else {
					_ = y.GetProgram().ImportTypeStaticMemberFromLib(prog, className, valName)
				}
			}
		} else {
			var lastErr error
			for idx := len(pkgNames) - 1; idx > 0; idx-- {
				className = strings.Join(pkgNames[idx:], ".")
				pkgName := strings.Join(pkgNames[:idx], ".")
				if lib, ok := y.GetProgram().GetLibrary(pkgName); ok && lib != nil {
					prog = lib
					break
				}
				if p, err := y.BuildDirectoryPackage(pkgNames[:idx], true); err == nil {
					prog = p
					break
				} else {
					lastErr = err
				}
			}
			if prog == nil {
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
			log.Warnf("Dependencies Missed: Import package %v but not found", pkgNames)
			continue
		}
		prog.PushEditor(y.GetEditor())
		if static {
			if !all {
				_ = y.GetProgram().ImportTypeFromLib(prog, className, pkgImport)
			}
		} else {
			if all {
				_ = y.GetProgram().ImportAll(prog)
			} else {
				_ = y.GetProgram().ImportTypeFromLib(prog, className, pkgImport)
			}
		}
		prog.PopEditor(false)
	}
}
