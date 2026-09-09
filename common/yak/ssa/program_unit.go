package ssa

import (
	"runtime"
	"strings"

	stdlog "github.com/yaklang/yaklang/common/log"
)

// BeginCompileUnit marks the start of a compile unit so deferred file builds
// capture this unitKey. Function/class LazyBuild is owned by the unit's
// library Program (see ProgramForCompileUnit), not by this string.
func (prog *Program) BeginCompileUnit(unitKey string) {
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.currentCompileUnit = unitKey
}

func (prog *Program) EndCompileUnit() {
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	app.currentCompileUnit = ""
}

func (prog *Program) CurrentCompileUnit() string {
	if prog == nil {
		return ""
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	return app.currentCompileUnit
}

// libraryNameFromCompileUnitKey maps a compile-unit key onto the library
// Program that owns that unit's functions and classes.
//
// Language-prefixed keys encode the package/namespace after the first colon
// (java:a.b → a.b, csharp:Foo.Bar → Foo.Bar). Unprefixed keys are used as-is
// (tests, explicit NewLibrary names). Directory/resource keys have no library
// name: their IR lives on the Application.
func libraryNameFromCompileUnitKey(unitKey string) string {
	if unitKey == "" {
		return ""
	}
	prefix, rest, ok := strings.Cut(unitKey, ":")
	if !ok {
		return unitKey
	}
	switch strings.ToLower(prefix) {
	case "dir", "resource", "unit":
		return ""
	default:
		return rest
	}
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
	if name := libraryNameFromCompileUnitKey(unitKey); name != "" {
		if lib, _ := app.GetLibrary(name); lib != nil {
			return lib
		}
		if lib, ok := app.UpStream.Get(name); ok && lib != nil {
			return lib
		}
		return nil
	}
	if lib, ok := app.UpStream.Get(unitKey); ok && lib != nil {
		return lib
	}
	// Directory-keyed units (python/go default) host IR on Application.
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

	// Iterate through all functions and release bodies for completed units
	app.Funcs.ForEach(func(funcKey string, fn *Function) bool {
		if fn == nil {
			return true
		}
		checkedFuncs++

		// Extract unit key from function key
		fnUnitKey := extractUnitKeyFromFunctionKey(funcKey)
		if fnUnitKey == "" {
			return true
		}

		// Debug first few functions
		if compileUnitMemoryDebugEnabled() && checkedFuncs <= 3 {
			log.Debugf("[split-compile] release func#%d key=%s unit=%s", checkedFuncs, funcKey, fnUnitKey)
		}

		// Check if this function belongs to a completed unit
		_, isCompleted := completedUnits[fnUnitKey]
		if !isCompleted {
			skippedNoMatch++
			return true
		}

		// Skip functions that should be kept for cross-unit references
		if shouldKeepFunctionForCrossUnitReference(fn) {
			skippedPublic++
			return true
		}

		// Release the function body. Keep EnterBlock/ExitBlock: they are valid
		// block ids that may still be referenced or flushed later, and zeroing
		// them would write empty block ids into the DB.
		if len(fn.Blocks) > 0 {
			blockCount := len(fn.Blocks)
			fn.Blocks = nil

			releasedFuncs++
			releasedBlocks += blockCount

			if compileUnitMemoryDebugEnabled() && releasedFuncs <= 3 {
				log.Debugf("[split-compile] released func=%s blocks=%d", funcKey, blockCount)
			}
		}
		return true
	})

	if compileUnitMemoryDebugEnabled() {
		log.Debugf("[split-compile] release summary checked=%d released=%d skipped_public=%d skipped_nomatch=%d blocks=%d",
			checkedFuncs, releasedFuncs, skippedPublic, skippedNoMatch, releasedBlocks)
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

	name := fn.GetName()
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return true
	}

	return false
}

// CheckMemoryPressure checks memory usage after each batch. It only reports;
// reclamation is left to the per-unit flush path and the adaptive GC
// (GOMEMLIMIT) started at compile start.
func (prog *Program) CheckMemoryPressure(batchIndex, totalBatches int) bool {
	if prog == nil {
		return false
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	heapMB := float64(m.HeapInuse) / (1024 * 1024)

	const (
		warningThresholdMB  = 2048
		criticalThresholdMB = 4096
	)

	if heapMB > criticalThresholdMB {
		log.Warnf("[split-compile] CRITICAL memory pressure detected: heap=%.1fMB batch=%d/%d - relying on per-unit flush + adaptive GC for reclaim",
			heapMB, batchIndex, totalBatches)
		return true
	}

	if heapMB > warningThresholdMB {
		log.Warnf("[split-compile] Memory pressure warning: heap=%.1fMB batch=%d/%d",
			heapMB, batchIndex, totalBatches)
	}

	return false
}
