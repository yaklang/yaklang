package ssa

import (
	"runtime"
	"runtime/debug"
	"strings"

	stdlog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// BeginCompileUnit marks the start of a compile unit: subsequent lazy/deferred
// builds capture this unitKey so per-unit runs (RunDeferredBuildsForUnits,
// LazyBuildForUnits) can scope work to a unit.
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

// ReleaseCompletedUnitMemory releases memory for completed compile units after flush.
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

		// Release function body (blocks and instructions)
		if len(fn.Blocks) > 0 {
			blockCount := len(fn.Blocks)
			fn.Blocks = nil
			fn.EnterBlock = 0
			fn.ExitBlock = 0

			releasedFuncs++
			releasedBlocks += blockCount

			if compileUnitMemoryDebugEnabled() && releasedFuncs <= 3 {
				log.Debugf("[split-compile] released func=%s blocks=%d", funcKey, blockCount)
			}
		}
		return true
	})

	if compileUnitMemoryDebugEnabled() {
		log.Debugf("[split-compile] release summary checked=%d released=%d skipped_public=%d skipped_nomatch=%d",
			checkedFuncs, releasedFuncs, skippedPublic, skippedNoMatch)
	}

	// Force GC to reclaim memory immediately
	if releasedFuncs > 0 {
		runtime.GC()
		if compileUnitMemoryDebugEnabled() {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			log.Debugf("[split-compile] released %d function bodies (%d blocks) heap=%.1fMB",
				releasedFuncs, releasedBlocks, float64(m.HeapInuse)/(1024*1024))
		}
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

// CheckMemoryPressure checks memory usage after each batch.
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
		log.Warnf("[split-compile] CRITICAL memory pressure detected: heap=%.1fMB batch=%d/%d - forcing aggressive cleanup",
			heapMB, batchIndex, totalBatches)
		prog.AggressiveClearMemory()
		return true
	}

	if heapMB > warningThresholdMB {
		log.Warnf("[split-compile] Memory pressure warning: heap=%.1fMB batch=%d/%d",
			heapMB, batchIndex, totalBatches)
	}

	return false
}

// AggressiveClearMemory forcefully clears all non-essential Program structures
// This is the NUCLEAR option for split compile memory control
// WARNING: This may break cross-unit references, only use after batch flush
func (prog *Program) AggressiveClearMemory() int64 {
	if prog == nil {
		return 0
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	beforeMB := m.HeapInuse / (1024 * 1024)

	// Clear all caches
	prog.cacheExternInstance = make(map[string]Value)
	prog.externType = make(map[string]Type)
	prog.ExternInstance = make(map[string]any)
	prog.ExternLib = make(map[string]map[string]any)

	// Clear offset map (can be rebuilt if needed)
	prog.OffsetMap = make(map[int]*OffsetItem)
	prog.OffsetSortedSlice = make([]int, 0)

	// NUCLEAR: Clear Blueprint map (type definitions)
	// Keep only GlobalVariables blueprint
	globalVars, _ := prog.Blueprint.Get("__GlobalVariables__")
	prog.Blueprint = omap.NewEmptyOrderedMap[string, *Blueprint]()
	if globalVars != nil {
		prog.Blueprint.Set("__GlobalVariables__", globalVars)
	}

	// NUCLEAR: Clear UpStream dependencies
	prog.UpStream = omap.NewEmptyOrderedMap[string, *Program]()
	prog.DownStream = make(map[string]*Program)

	// Note: Do NOT clear Funcs here - lazy builders need access to functions
	// defined in earlier batches. Only clear constants and exports.

	// Also clear constants - these accumulate heavily!
	prog.Consts = make(map[string]Value)
	prog.ExportValue = make(map[string]Value)
	prog.ExportType = make(map[string]Type)

	prog.deferredBuilds = omap.NewEmptyOrderedMap[string, *deferredBuildTask]()

	// NEW: Clear editor stack - holds file content
	prog.editorStack = omap.NewEmptyOrderedMap[string, *memedit.MemEditor]()

	// CRITICAL: Clear diagnostics recorder - this accumulates trace steps heavily!
	// Found via heap profiling: diagnostics.appendStep grows by 587 KB per batch
	if prog.diagnosticsRecorder != nil {
		prog.diagnosticsRecorder = nil
	}

	// CRITICAL: Clear FileList - file hash mappings
	prog.FileList = make(map[string]string)
	prog.LibraryFile = make(map[string][]string)

	// Note: Do NOT clear CurrentIncludingStack here - it's needed by lazy builders
	// that may run after this cleanup

	// Force GC multiple times to ensure everything is collected
	runtime.GC()
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()

	runtime.ReadMemStats(&m)
	afterMB := m.HeapInuse / (1024 * 1024)

	freedMB := int64(beforeMB - afterMB)
	if compileUnitMemoryDebugEnabled() {
		log.Debugf("[split-compile] aggressive clear heap=%dMB->%dMB freed=%dMB", beforeMB, afterMB, freedMB)
	}

	return freedMB
}
