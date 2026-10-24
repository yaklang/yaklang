package ssa

import (
	"runtime"
	"runtime/debug"
	"strings"

	stdlog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memorybudget"
)

// AppendLibraryFile records a frontend's library-to-file relation on the
// application. Compile-unit prehandlers may call this from different owners.
func (prog *Program) AppendLibraryFile(name, filename string) {
	if prog == nil || name == "" || filename == "" {
		return
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.libraryFileMu.Lock()
	if app.LibraryFile == nil {
		app.LibraryFile = make(map[string][]string)
	}
	app.LibraryFile[name] = append(app.LibraryFile[name], filename)
	app.libraryFileMu.Unlock()
}

// LibraryFiles returns a snapshot so readers do not share the mutable slice.
func (prog *Program) LibraryFiles(name string) []string {
	if prog == nil || name == "" {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.libraryFileMu.RLock()
	files := append([]string(nil), app.LibraryFile[name]...)
	app.libraryFileMu.RUnlock()
	return files
}

// BeginCompileUnit records the registering worker's unitKey. A Program may be
// visited by another worker (for example through an import); that visitor must
// not inherit or clear this worker's context. Only deferred task registration
// consults this context, not instruction emission or the library body owner.
func (prog *Program) BeginCompileUnit(unitKey string) {
	if prog == nil {
		return
	}
	if unitKey == "" {
		prog.EndCompileUnit()
		return
	}
	gid := currentGoroutineID()
	prog.compileUnitContexts.Store(gid, unitKey)
	// A frontend may create another library Program inside PreHandlerProject
	// (for example go2ssa switches from the directory library to the declared
	// package library). Mirroring the goroutine's unit on the application lets
	// that new library's CurrentCompileUnit find it instead of returning "".
	if app := prog.GetApplication(); app != nil && app != prog {
		app.compileUnitContexts.Store(gid, unitKey)
	}
}

func (prog *Program) EndCompileUnit() {
	if prog == nil {
		return
	}
	gid := currentGoroutineID()
	prog.compileUnitContexts.Delete(gid)
	if app := prog.GetApplication(); app != nil && app != prog {
		app.compileUnitContexts.Delete(gid)
	}
}

func (prog *Program) CurrentCompileUnit() string {
	if prog == nil {
		return ""
	}
	gid := currentGoroutineID()
	if unit, ok := prog.compileUnitContexts.Load(gid); ok {
		return unit.(string)
	}
	// Deferred helpers run with the application context restored on their own
	// goroutine. Never fall back to another worker's active library context.
	if app := prog.GetApplication(); app != nil && app != prog {
		if unit, ok := app.compileUnitContexts.Load(gid); ok {
			return unit.(string)
		}
	}
	return ""
}

// EnsureLibraryForCompileUnit returns the Program that will own this unit.
// Library-backed units are created before concurrent pre-handlers start so
// NewLibrary is not called from worker goroutines.
func (prog *Program) EnsureLibraryForCompileUnit(unitKey string) *Program {
	if prog == nil {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	name := prog.libraryNameForCompileUnit(unitKey)
	if name == "" {
		return app
	}
	if lib, _ := app.GetLibrary(name); lib != nil {
		return lib
	}
	if lib, ok := app.UpStream.Get(name); ok && lib != nil {
		return lib
	}
	return app.NewLibrary(name, strings.Split(name, "."))
}

// GetOrCreateUnitBuilder returns a fully initialized builder for this unit.
// Library builders inherit build support from the application builder so
// CurrentBlock, language config and parent scope are present before PreHandler.
func (prog *Program) GetOrCreateUnitBuilder(unitKey string, parent *FunctionBuilder) *FunctionBuilder {
	if prog == nil {
		return parent
	}
	name := prog.libraryNameForCompileUnit(unitKey)
	if name == "" {
		if parent != nil {
			return parent
		}
		return prog.GetAndCreateFunctionBuilder(string(MainFunctionName), string(MainFunctionName))
	}
	owner := prog.EnsureLibraryForCompileUnit(unitKey)
	if owner == nil {
		return parent
	}
	owner.SetPackageName(name)
	fun := owner.GetAndCreateFunction(name, string(MainFunctionName))
	if fun == nil {
		return parent
	}
	if fun.builder != nil {
		if parent != nil {
			fun.builder.SetBuildSupport(parent)
			fun.builder.DefineFunc = parent.DefineFunc
			fun.builder.ctx = parent.ctx
		}
		if fun.builder.CurrentBlock == nil {
			if block, ok := fun.GetBasicBlockByID(fun.EnterBlock); ok && block != nil {
				fun.builder.CurrentBlock = block
				block.restoreScopeIfMissing()
			}
		}
		return fun.builder
	}
	builder := NewBuilder(nil, fun, nil)
	if parent != nil {
		builder.SetBuildSupport(parent)
		builder.DefineFunc = parent.DefineFunc
		builder.ctx = parent.ctx
	}
	return builder
}

// libraryNameFromCompileUnitKey maps a compile-unit key onto the library
// Program that owns that unit's functions and classes.
//
// Language-prefixed keys encode the package/namespace after the first colon
// (java:a.b → a.b, csharp:Foo.Bar → Foo.Bar, php:App.Http → App.Http).
// Directory keys (dir:helper/db) become library names so Python/JS/TS
// directory units can drain concurrently. Go uses its declared package name;
// PHP keeps the serial application drain.
// Resource keys and the project-root directory stay on the Application.
func libraryNameFromCompileUnitKey(unitKey string) string {
	if unitKey == "" {
		return ""
	}
	prefix, rest, ok := strings.Cut(unitKey, ":")
	if !ok {
		return unitKey
	}
	switch strings.ToLower(prefix) {
	case "resource":
		return ""
	case "dir", "unit":
		rest = strings.Trim(strings.ReplaceAll(rest, "\\", "/"), "/")
		if rest == "" || rest == "." {
			return ""
		}
		return strings.ReplaceAll(rest, "/", ".")
	default:
		return strings.ReplaceAll(rest, "\\", ".")
	}
}

// A frontend that sets DeclaredLibrary owns the library itself. Declaration
// registration stays on the application; ProgramForCompileUnit maps the body
// build onto LibraryName.
func (prog *Program) libraryNameForCompileUnit(unitKey string) string {
	if unit := prog.compileUnitByKey(unitKey); unit != nil && unit.DeclaredLibrary {
		return ""
	}
	return libraryNameFromCompileUnitKey(unitKey)
}

func (prog *Program) compileUnitByKey(unitKey string) *CompileUnit {
	if prog == nil || unitKey == "" {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	for _, unit := range app.CompileUnits {
		if unit != nil && unit.Key == unitKey {
			return unit
		}
	}
	return nil
}

// ProgramsForCompileUnit returns every library this unit must build.
// A Go directory that holds package foo and package foo_test owns both
// libraries. Draining only the application (LibraryName empty) left every
// function body pending until the final Finish.
func (prog *Program) ProgramsForCompileUnit(unitKey string) []*Program {
	if prog == nil || unitKey == "" {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	unit := app.compileUnitByKey(unitKey)
	if unit == nil || !unit.DeclaredLibrary || len(unit.LibraryNames) == 0 {
		if one := prog.ProgramForCompileUnit(unitKey); one != nil {
			return []*Program{one}
		}
		return nil
	}
	out := make([]*Program, 0, len(unit.LibraryNames))
	seen := map[*Program]struct{}{}
	for _, name := range unit.LibraryNames {
		if name == "" {
			continue
		}
		lib, _ := app.GetLibrary(name)
		if lib == nil {
			continue
		}
		if _, ok := seen[lib]; ok {
			continue
		}
		seen[lib] = struct{}{}
		out = append(out, lib)
	}
	if len(out) == 0 {
		return []*Program{app}
	}
	return out
}

// ProgramForCompileUnit returns the Program that owns this compile unit.
// A unit is that library: it already contains the package's functions and
// classes, so LazyBuildForUnits drains this Program rather than walking
// Application tagged with interned ids.
func (prog *Program) ProgramForCompileUnit(unitKey string) *Program {
	if prog == nil || unitKey == "" {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	if unit := app.compileUnitByKey(unitKey); unit != nil && unit.DeclaredLibrary {
		if len(unit.LibraryNames) > 0 {
			if lib, _ := app.GetLibrary(unit.LibraryNames[0]); lib != nil {
				return lib
			}
		}
		if unit.LibraryName != "" {
			if lib, _ := app.GetLibrary(unit.LibraryName); lib != nil {
				return lib
			}
		}
		return app
	}
	if name := prog.libraryNameForCompileUnit(unitKey); name != "" {
		if lib, _ := app.GetLibrary(name); lib != nil {
			return lib
		}
		if lib, ok := app.UpStream.Get(name); ok && lib != nil {
			return lib
		}
		prefix, _, _ := strings.Cut(unitKey, ":")
		switch strings.ToLower(prefix) {
		case "dir", "unit":
			return app
		}
		return nil
	}
	if lib, ok := app.UpStream.Get(unitKey); ok && lib != nil {
		return lib
	}
	return app
}

// ReleaseCompletedUnitMemory releases the completed compile units' function
// bodies and clears program-level caches that the per-unit flush path no
// longer needs. Called once per unit at the end of FlushCompileUnit; the GC
// itself runs once in FlushCompileUnit after this returns.
func (prog *Program) ReleaseCompletedUnitMemory(unitKeys []string) int {
	if prog == nil || len(unitKeys) == 0 {
		return 0
	}

	app := prog.GetApplication()
	if app == nil {
		app = prog
	}

	releasedFuncs := 0
	releasedBlocks := 0
	checkedFuncs := 0
	skippedPublic := 0
	skippedNoMatch := 0
	skippedUnfinished := 0

	// Build a set of completed unit keys for O(1) lookup
	completedUnits := make(map[string]struct{}, len(unitKeys))
	for _, key := range unitKeys {
		completedUnits[key] = struct{}{}
	}

	if compileUnitMemoryDebugEnabled() {
		log.Debugf("[split-compile] release start units=%d total_funcs=%d", len(unitKeys), app.Funcs.Len())
		if len(unitKeys) > 0 {
			log.Debugf("[split-compile] release unit keys sample=%v", unitKeys[:min(3, len(unitKeys))])
		}
	}

	releaseOne := func(funcKey string, fn *Function) {
		if fn == nil {
			return
		}
		checkedFuncs++

		// Prefer the compile unit recorded when the function was created: the
		// key parser below only understands the Java package naming scheme and
		// returns "" for Python/Go/C function names, which silently disabled the
		// body release for those languages.
		fnUnitKey := fn.compileUnitKey
		if fnUnitKey == "" {
			fnUnitKey = extractUnitKeyFromFunctionKey(funcKey)
		}
		if fnUnitKey == "" {
			return
		}

		// Debug first few functions
		if compileUnitMemoryDebugEnabled() && checkedFuncs <= 3 {
			log.Debugf("[split-compile] release func#%d key=%s unit=%s", checkedFuncs, funcKey, fnUnitKey)
		}

		// Check if this function belongs to a completed unit
		_, isCompleted := completedUnits[fnUnitKey]
		if !isCompleted {
			skippedNoMatch++
			return
		}

		// Only a finished body can be dropped. A unit can be marked complete
		// while a lazily registered function of that unit still waits for its
		// Finish(); dropping its Blocks early makes Finish read Blocks[0] and
		// panic, which then discards the whole compiled program.
		if !fn.IsFinished() {
			skippedUnfinished++
			return
		}

		// Skip functions that should be kept for cross-unit references
		if shouldKeepFunctionForCrossUnitReference(fn) {
			skippedPublic++
			return
		}

		// Do not clear fn.Blocks here. The slice is only block ids, and
		// ReleasePersistedOrdinaryValues walks those ids to unlink scope
		// values. Clearing it first makes that walk a no-op, so the values
		// stay reachable for the rest of the compile. The id list is dropped
		// after that walk. EnterBlock/ExitBlock stay either way: they are
		// valid block ids that may still be flushed.
		if len(fn.Blocks) > 0 {
			releasedFuncs++
			releasedBlocks += len(fn.Blocks)

			if compileUnitMemoryDebugEnabled() && releasedFuncs <= 3 {
				log.Debugf("[split-compile] release eligible func=%s blocks=%d", funcKey, len(fn.Blocks))
			}
		}
	}

	// File/directory units live in library Programs ("dir:pkg" -> library
	// "pkg"), so the application's Funcs is not the whole program. Walk the
	// upstream libraries too, otherwise every Go/Python/Java unit body stayed
	// resident for the whole compile.
	app.Funcs.ForEach(func(funcKey string, fn *Function) bool {
		releaseOne(funcKey, fn)
		return true
	})
	if app.UpStream != nil {
		app.UpStream.ForEach(func(_ string, lib *Program) bool {
			if lib != nil && lib != app {
				lib.Funcs.ForEach(func(funcKey string, fn *Function) bool {
					releaseOne(funcKey, fn)
					return true
				})
			}
			return true
		})
	}

	if compileUnitMemoryDebugEnabled() {
		log.Debugf("[split-compile] release summary checked=%d released=%d skipped_public=%d skipped_unfinished=%d skipped_nomatch=%d blocks=%d",
			checkedFuncs, releasedFuncs, skippedPublic, skippedUnfinished, skippedNoMatch, releasedBlocks)
	}

	return releasedFuncs
}

func compileUnitMemoryDebugEnabled() bool {
	return log.Level >= stdlog.DebugLevel
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractUnitKeyFromFunctionKey extracts the compile unit key from a function key.
func extractUnitKeyFromFunctionKey(funcKey string) string {
	parts := strings.SplitN(funcKey, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	lang := parts[0]
	rest := parts[1]

	pathParts := strings.Split(rest, ".")
	if len(pathParts) <= 2 {
		return funcKey
	}

	// Take all but the last 2 parts (class + method)
	packageParts := pathParts[:len(pathParts)-2]
	packageName := strings.Join(packageParts, ".")

	return lang + ":" + packageName
}

// shouldKeepFunctionForCrossUnitReference determines if a function should be kept.
func shouldKeepFunctionForCrossUnitReference(fn *Function) bool {
	if fn == nil {
		return false
	}

	if fn.IsExtern() {
		return true
	}

	// A method is reached through the owning type's member map, not as a
	// top-level cross-unit symbol. The Function object stays referenced by the
	// blueprint, so its body and parameters can be reloaded by ID. Without this
	// every method of every class stays pinned for the whole program as soon as
	// its generated name starts with the (capitalised) class name, which is the
	// Python/Java method naming convention.
	if fn.IsMethod() || fn.GetMethodName() != "" {
		return false
	}

	name := fn.GetName()
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return true
	}

	return false
}

// shouldReturnIdleHeap is true when the runtime is holding more unused
// heap than the live heap, and that unused part is at least 1GiB.
// GOMEMLIMIT on a large machine lets RSS stay at the high-water mark long
// after a batch has dropped its values, so the next batch has no headroom.
func shouldReturnIdleHeap(inuse, unreleased uint64) bool {
	const floor = 1 << 30
	return unreleased > floor && unreleased > inuse/2
}

func idleHeapUnreleased(m runtime.MemStats) uint64 {
	if m.HeapIdle < m.HeapReleased {
		return 0
	}
	return m.HeapIdle - m.HeapReleased
}

// CheckMemoryPressure checks memory usage after each batch. It reports
// pressure and, when the runtime is holding a large idle heap, returns
// those pages so RSS follows the live set instead of the high-water mark.
func (prog *Program) CheckMemoryPressure(batchIndex, totalBatches int) bool {
	if prog == nil {
		return false
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	warning, critical := memorybudget.PressureThresholds(memorybudget.Total(), debug.SetMemoryLimit(-1))
	// Match the Go memory-limit accounting, which includes stacks and runtime
	// metadata rather than only heap objects. This remains diagnostic only.
	managed := m.Sys - m.HeapReleased
	managedMB := float64(managed) / (1024 * 1024)
	warningMB, criticalMB := float64(warning)/(1024*1024), float64(critical)/(1024*1024)

	pressured := false
	if managed > uint64(critical) {
		log.Warnf("[split-compile] high memory pressure: managed=%.1fMB warning=%.1fMB critical=%.1fMB batch=%d/%d",
			managedMB, warningMB, criticalMB, batchIndex, totalBatches)
		pressured = true
	} else if managed > uint64(warning) {
		log.Warnf("[split-compile] memory pressure: managed=%.1fMB warning=%.1fMB critical=%.1fMB batch=%d/%d",
			managedMB, warningMB, criticalMB, batchIndex, totalBatches)
	}

	if shouldReturnIdleHeap(m.HeapInuse, idleHeapUnreleased(m)) {
		before := idleHeapUnreleased(m)
		debug.FreeOSMemory()
		runtime.ReadMemStats(&m)
		log.Infof("[split-compile] returned idle heap: before=%.1fMB after=%.1fMB inuse=%.1fMB batch=%d/%d",
			float64(before)/(1024*1024), float64(idleHeapUnreleased(m))/(1024*1024),
			float64(m.HeapInuse)/(1024*1024), batchIndex, totalBatches)
	}

	return pressured
}
