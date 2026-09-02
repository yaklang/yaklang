package python2ssa

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// pythonModuleBuildDepthLimit guards against pathological nesting (deep import
// chains each importing the next): beyond this depth the fallback gives up and
// the caller binds the placeholder instead of recursing further.
//
// Note the counter is per-file-builder: each nested build creates a fresh
// builder, so cross-module chains (A imports B imports C ...) are bounded by
// the busy-set cycle guard below and CPython-scale import depths, not by
// this counter. It protects the same-builder recursion path.
const pythonModuleBuildDepthLimit = 32

// pyBusyImportBuilds tracks module files currently being compiled by the
// need-driven import fallback. It is keyed on the application program so
// recursive import cycles (A imports B imports A) terminate: a file already
// busy is skipped instead of recursing, and the caller falls back to its
// placeholder path. The zero value is usable.
var pyBusyImportBuilds sync.Map // *ssa.Program (application) -> *sync.Map (fileHash -> struct{})

// pyBeginImportBuild marks a module file as being compiled on demand. It
// returns false when the file is already busy (import cycle in progress).
func pyBeginImportBuild(app *ssa.Program, fileHash string) bool {
	if app == nil || fileHash == "" {
		return false
	}
	set, _ := pyBusyImportBuilds.LoadOrStore(app, &sync.Map{})
	busy := set.(*sync.Map)
	if _, busy := busy.LoadOrStore(fileHash, struct{}{}); busy {
		return false
	}
	return true
}

// pyEndImportBuild clears the busy mark after the on-demand compile finished.
func pyEndImportBuild(app *ssa.Program, fileHash string) {
	if app == nil || fileHash == "" {
		return
	}
	if set, ok := pyBusyImportBuilds.Load(app); ok {
		set.(*sync.Map).Delete(fileHash)
	}
}

// pythonImportFallbackEnabled gates the need-driven fallback; it is disabled
// while the skeleton pass runs (PreHandler) because the batch pipeline compiles
// every scheduled file anyway and the deferred import statements only execute
// after the whole batch's skeletons have published their exports.
func (b *singleFileBuilder) pythonImportFallbackEnabled() bool {
	return b != nil && !b.PreHandler()
}

// ensurePythonModuleBuilt is the need-driven fallback for `bindImportedName`:
// when an imported module name is not resolvable from any library yet, compile
// the target file on demand (PHP-include style, see ssa.BuildFilePackage) so
// its exports are visible to the importer — instead of binding an Any
// placeholder that only gets patched after the whole project finishes
// (fixImportCallback loses mid-compile type inference).
//
// sourceName is the dotted module path as written in the import
// ("pkg.mod" absolute, ".sibling" / "..pkg.mod" relative). It is resolved
// against the project filesystem (app.Loader); relative paths anchor on the
// importing file's directory. Only files that exist in the project compile —
// third-party names (e.g. `from frappe import get_doc`) fall through to the
// placeholder path unchanged.
//
// The nested compile follows the PHP rules (see ssa.BuildFilePackage):
//   - the per-file UpStream hash cache guarantees each file compiles once;
//   - pyBeginImportBuild guards cyclic imports (A imports B imports A);
//   - the nested path never touches Begin/EndCompileUnit and never registers
//     deferred tasks (they would escape the compile-unit batch snapshot) —
//     PreHandler is off inside this path, so registerPythonFileBuild and
//     registerPythonSingleInputBuild skip;
//   - the sub-program builds under its own main function builder, and its
//     module-level exports (defs via predeclare, variables via the
//     assignToTarget publish) land in the module's virtual library, which
//     lookupPyImportedValue re-queries.
func (b *singleFileBuilder) ensurePythonModuleBuilt(sourceName string) {
	if b == nil || sourceName == "" || b.FunctionBuilder == nil {
		return
	}
	if !b.pythonImportFallbackEnabled() {
		return
	}
	prog := b.GetProgram()
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil || app.Build == nil {
		// The application program owns the editor stack and UpStream cache;
		// without it the nested build cannot register its file.
		return
	}
	if b.includeDepth > pythonModuleBuildDepthLimit {
		return
	}
	loader := app.Loader
	if loader == nil {
		return
	}
	fsys := loader.GetFilesysFileSystem()
	if fsys == nil {
		return
	}

	filePath, fileHash := pyResolveModuleFile(fsys, app, b.GetEditor(), sourceName)
	if filePath == "" {
		return
	}

	// Already compiled once: the UpStream cache holds the sub-program.
	if subProg, ok := app.UpStream.Get(fileHash); ok && subProg != nil {
		return
	}
	// A recursive import of a module still being compiled (A -> B -> A): stop
	// here and let the caller bind the placeholder for the cycle edge.
	if !pyBeginImportBuild(app, fileHash) {
		return
	}
	defer pyEndImportBuild(app, fileHash)

	src, err := fsys.ReadFile(filePath)
	if err != nil || len(src) == 0 {
		return
	}
	// Python sources only (the language builder knows no other module form).
	if !(&SSABuilder{}).FilterFile(filePath) {
		return
	}

	log.Debugf("[py-import-fallback] compiling module on demand: %s (%s)", sourceName, filePath)
	editor := app.CreateEditor(src, filePath)
	ast, err := FrontendWithCache(string(src))
	if err != nil {
		log.Debugf("[py-import-fallback] parse %s failed: %v", filePath, err)
		return
	}
	subProg := app.GetSubProgram(fileHash)
	builder := subProg.GetAndCreateFunctionBuilder("", string(ssa.MainFunctionName))
	b.includeDepth++
	defer func() { b.includeDepth-- }()
	if err := app.Build(ast, editor, builder); err != nil {
		log.Debugf("[py-import-fallback] build %s failed: %v", filePath, err)
		return
	}
	subProg.LazyBuild()
	builder.Finish()
	app.UpStream.Set(fileHash, subProg)
	log.Debugf("[py-import-fallback] module compiled: %s", sourceName)
}

// pyResolveModuleFile maps a dotted module path to a project file path plus
// its pure source hash, or ("", "") when no such module exists. Relative names
// (one leading dot = the importer's own package, two = its parent, ...) anchor
// on the importer's directory. `pkg.mod` prefers pkg/mod.py, then
// pkg/mod/__init__.py; `pkg` prefers pkg/__init__.py, then pkg.py. Absolute
// names also try successively dropping leading parts (sys.path semantics: the
// project root acts as a sys.path entry, so "backend.pkg.mod" finds
// "pkg/mod.py" when the scan root is "backend/").
func pyResolveModuleFile(
	fsys filesys_interface.FileSystem,
	app *ssa.Program,
	importerEditor *memedit.MemEditor,
	sourceName string,
) (string, string) {
	if fsys == nil || sourceName == "" {
		return "", ""
	}
	// Split the relative prefix (leading dots) from the dotted suffix.
	dots := 0
	for dots < len(sourceName) && sourceName[dots] == '.' {
		dots++
	}
	suffix := sourceName[dots:]
	if dots == 0 && suffix == "" {
		return "", ""
	}

	// Base directory parts for the module path.
	var dirParts []string
	if dots > 0 {
		if importerEditor == nil {
			return "", ""
		}
		dir := strings.Trim(strings.ReplaceAll(importerEditor.GetFolderPath(), "\\", "/"), "/")
		parts := []string{}
		if dir != "" {
			parts = strings.Split(dir, "/")
		}
		// The first dot refers to the importer's own package directory, so
		// ascend only dots-1 levels.
		up := dots - 1
		if up > len(parts) {
			return "", ""
		}
		dirParts = parts[:len(parts)-up]
	}

	var suffixParts []string
	if suffix != "" {
		suffixParts = strings.Split(suffix, ".")
	}

	moduleParts := append(append([]string{}, dirParts...), suffixParts...)
	join := func(parts ...string) string {
		clean := make([]string, 0, len(parts))
		for _, p := range parts {
			if p == "" || p == "." {
				continue
			}
			if p == ".." {
				if len(clean) > 0 {
					clean = clean[:len(clean)-1]
				}
				continue
			}
			clean = append(clean, p)
		}
		return strings.Join(clean, "/")
	}

	// Candidate paths, most specific first: a dotted module pkg.mod is
	// pkg/mod.py, then pkg/mod/__init__.py; a plain pkg prefers pkg/__init__.py;
	// a bare relative import targets the package directory's __init__.py.
	// Absolute names mimic Python's sys.path search: the full path first,
	// then leading parts progressively dropped (an import like
	// "backend.pkg.mod" finds "pkg/mod.py" when the scan root is "backend/" —
	// the project root acts as a sys.path entry).
	var candidates []string
	if len(suffixParts) > 0 {
		for offset := 0; offset <= len(moduleParts)-1 && (dots == 0 || offset == 0); offset++ {
			parts := moduleParts[offset:]
			last := parts[len(parts)-1]
			candidates = append(candidates,
				join(append(append([]string{}, parts[:len(parts)-1]...), last+".py")...),
				join(append(append([]string{}, parts...), "__init__.py")...),
			)
		}
	} else {
		candidates = append(candidates, join(append(append([]string{}, dirParts...), "__init__.py")...))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := fsys.Stat(candidate); err == nil && info != nil && !info.IsDir() {
			// Hash the real file content: CreateEditor(nil, ...) would hash an
			// empty source and never match the UpStream cache of a build.
			src, err := fsys.ReadFile(candidate)
			if err != nil || len(src) == 0 {
				continue
			}
			editor := app.CreateEditor(src, candidate, false)
			return candidate, editor.GetPureSourceHash()
		}
	}
	return "", ""
}