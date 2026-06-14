package ssa

import (
	"fmt"
	"runtime"
	"strings"

	yaklog "github.com/yaklang/yaklang/common/log"
)

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

	fmt.Printf("\n[RELEASE-TRACE] Starting release: units=%d, totalFuncs=%d\n", len(unitKeys), app.Funcs.Len())
	if len(unitKeys) > 0 {
		fmt.Printf("[RELEASE-TRACE] First 3 unit keys: %v\n", unitKeys[:min(3, len(unitKeys))])
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
		if checkedFuncs <= 3 {
			fmt.Printf("[RELEASE-TRACE] Func#%d: key=%s → unitKey=%s\n", checkedFuncs, funcKey, fnUnitKey)
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

			if releasedFuncs <= 3 {
				fmt.Printf("[RELEASE-TRACE] Released func: %s (blocks=%d)\n", funcKey, blockCount)
			}
		}
		return true
	})

	fmt.Printf("[RELEASE-TRACE] Summary: checked=%d released=%d skipped_public=%d skipped_nomatch=%d\n",
		checkedFuncs, releasedFuncs, skippedPublic, skippedNoMatch)

	// Force GC to reclaim memory immediately
	if releasedFuncs > 0 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("[RELEASE-SUCCESS] Released %d function bodies (%d blocks) - heap=%.1fMB\n",
			releasedFuncs, releasedBlocks, float64(m.HeapInuse)/(1024*1024))
	}

	return releasedFuncs
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
		yaklog.Warnf("[split-compile] CRITICAL memory pressure detected: heap=%.1fMB batch=%d/%d - forcing aggressive cleanup",
			heapMB, batchIndex, totalBatches)
		prog.ForceCleanupNonExportedFunctions()
		return true
	}

	if heapMB > warningThresholdMB {
		yaklog.Warnf("[split-compile] Memory pressure warning: heap=%.1fMB batch=%d/%d",
			heapMB, batchIndex, totalBatches)
	}

	return false
}

// ForceCleanupNonExportedFunctions aggressively releases all non-exported function bodies.
func (prog *Program) ForceCleanupNonExportedFunctions() int {
	if prog == nil {
		return 0
	}

	app := prog.GetApplication()
	if app == nil {
		app = prog
	}

	released := 0
	app.Funcs.ForEach(func(key string, fn *Function) bool {
		if fn == nil {
			return true
		}

		if fn.IsExtern() {
			return true
		}

		name := fn.GetName()
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			return true
		}

		if len(fn.Blocks) > 0 {
			fn.Blocks = nil
			fn.EnterBlock = 0
			fn.ExitBlock = 0
			released++
		}
		return true
	})

	if released > 0 {
		runtime.GC()
		yaklog.Infof("[split-compile] ForceCleanup released %d function bodies", released)
	}

	return released
}
