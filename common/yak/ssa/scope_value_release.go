package ssa

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
)

// batchReleaseDisabledForTest is set only from tests that compare persisted
// IR with the post-batch release turned off. Production leaves it false.
var batchReleaseDisabledForTest atomic.Bool

func SetBatchReleaseDisabledForTest(disabled bool) {
	batchReleaseDisabledForTest.Store(disabled)
}

// ReleasePersistedOrdinaryValues drops strong scope references to ordinary
// instructions that have already been saved. The variable keeps the id and
// GetValue loads it again. Externs, unfinished functions, closure-captured
// values, and boundary instructions stay as objects. A finished exported
// function keeps its own object and parameters, but its saved locals follow
// the same id-reload path: the capitalized name is the cross-unit callable,
// not a reason to retain every local.
//
// Callers must run this only after the batch workers have joined and the
// instruction saver barrier has returned. It does not guess compile-unit
// names and does not clear fn.Blocks.
func (prog *Program) ReleasePersistedOrdinaryValues() int {
	if batchReleaseDisabledForTest.Load() || prog == nil || prog.Cache == nil || prog.Cache.instructions == nil {
		return 0
	}
	if prog.Cache.instructions.mode != ProgramCacheDBWrite {
		return 0
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	released := 0
	seen := map[*Program]struct{}{}
	var visit func(*Program)
	visit = func(p *Program) {
		if p == nil {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		released += releaseProgramOrdinaryValues(p)
		released += releasePersistedOffsetValues(p)
		if p.UpStream == nil {
			return
		}
		p.UpStream.ForEach(func(_ string, child *Program) bool {
			visit(child)
			return true
		})
	}
	visit(app)
	return released
}

// releasePersistedOffsetValues drops offset pointers to ordinary values that
// have already been saved. The id stays, and GetValue reloads it. Exported
// parameters and closure captures remain objects for cross-unit binding.
func releasePersistedOffsetValues(prog *Program) int {
	if prog == nil || prog.Cache == nil || prog.Cache.instructions == nil {
		return 0
	}
	// Copy the items out before inspecting them. valueCapturedByClosure on a
	// LazyInstruction reloads the row, and that reload assigns the value back
	// through SetOffsetValue, which takes offsetMu. Holding the lock across
	// that call deadlocks the compile thread.
	prog.offsetMu.Lock()
	items := make([]*OffsetItem, 0, len(prog.OffsetMap))
	for _, item := range prog.OffsetMap {
		if item != nil && !utils.IsNil(item.value) {
			items = append(items, item)
		}
	}
	prog.offsetMu.Unlock()

	released := 0
	for _, item := range items {
		if valueCapturedByClosure(item.value) {
			continue
		}
		if shouldKeepCompileUnitBoundaryResident(item.value) && !canSpillFinishedPrivateBlock(item.value) && !canSpillFinishedFunctionMember(item.value) {
			continue
		}
		id := item.value.GetId()
		if id <= 0 || !prog.Cache.instructions.isPersisted(id) {
			continue
		}
		prog.offsetMu.Lock()
		if utils.IsNil(item.value) {
			prog.offsetMu.Unlock()
			continue
		}
		if item.valueID == 0 {
			item.valueID = id
		}
		if item.program == nil {
			item.program = prog
		}
		item.value = nil
		prog.offsetMu.Unlock()
		released++
	}
	return released
}

func releaseProgramOrdinaryValues(prog *Program) int {
	if prog == nil || prog.Cache == nil || prog.Cache.instructions == nil {
		return 0
	}
	reload := func(id int64) (Value, bool) {
		// Bounded reload cache, not Cache.Get. Cache.Get puts a miss back
		// into the unbounded resident map and would undo the release.
		inst := prog.Cache.instructions.reloadPersistedInstruction(id)
		if utils.IsNil(inst) {
			return nil, false
		}
		value, ok := ToValue(inst)
		return value, ok && !utils.IsNil(value)
	}
	released := 0
	visitedScopes := map[*ScopeInstance]struct{}{}
	prog.forEachFunction(func(fn *Function) {
		if fn == nil || !fn.finished || fn.IsExtern() {
			return
		}
		if fn.builder != nil && fn.builder.CurrentBlock != nil && fn.builder.CurrentBlock.ScopeTable != nil {
			released += releaseScopeOrdinaryValues(prog, fn.builder.CurrentBlock.ScopeTable, visitedScopes, reload)
		}
		for _, id := range fn.Blocks {
			inst, ok := prog.Cache.instructions.residentInstruction(id)
			if !ok || utils.IsNil(inst) {
				continue
			}
			block, ok := ToBasicBlock(inst)
			if !ok || block == nil || block.ScopeTable == nil {
				continue
			}
			released += releaseScopeOrdinaryValues(prog, block.ScopeTable, visitedScopes, reload)
		}
		// The id list has been walked. Dropping the builder of a private
		// function is not enough: its BasicBlocks stay in the resident cache,
		// and an exported function keeps both the builder and the blocks. The
		// scope tables on those blocks are the version maps that otherwise live
		// until compile end. A scope that still holds a closure capture stays,
		// and so does a blueprint container that later batches still emit into.
		if !functionScopeRetainsCapture(prog, fn) && !isLiveContainerFunction(fn) {
			clearFunctionBodyScopes(prog, fn)
			fn.builder = nil
		} else if !shouldKeepFunctionForCrossUnitReference(fn) {
			fn.builder = nil
		}
		// An exported function keeps its block ids for cross-unit lookup.
		// EnterBlock/ExitBlock stay set either way.
		if !shouldKeepFunctionForCrossUnitReference(fn) {
			fn.Blocks = nil
		}
	})
	return released
}

func functionScopeRetainsCapture(prog *Program, fn *Function) bool {
	if fn == nil {
		return false
	}
	found := false
	visit := func(scope ScopeIF) {
		if found || scope == nil {
			return
		}
		current, ok := scope.(*ScopeInstance)
		if !ok || current == nil {
			return
		}
		for _, latest := range current.GetAllVariables() {
			if latest == nil {
				continue
			}
			for _, versioned := range current.GetAllVariablesByName(latest.GetName(), true) {
				variable, ok := versioned.(*Variable)
				if !ok || variable == nil || utils.IsNil(variable.Value) {
					continue
				}
				if valueCapturedByClosure(variable.Value) {
					found = true
					return
				}
			}
		}
	}
	if fn.builder != nil && fn.builder.CurrentBlock != nil {
		visit(fn.builder.CurrentBlock.ScopeTable)
	}
	if prog != nil && prog.Cache != nil && prog.Cache.instructions != nil {
		for _, id := range fn.Blocks {
			inst, ok := prog.Cache.instructions.residentInstruction(id)
			if !ok || utils.IsNil(inst) {
				continue
			}
			block, ok := ToBasicBlock(inst)
			if !ok || block == nil {
				continue
			}
			visit(block.ScopeTable)
		}
	}
	return found
}

func isLiveContainerFunction(fn *Function) bool {
	if fn == nil {
		return false
	}
	prog := fn.GetProgram()
	if prog == nil || prog.GlobalVariablesBlueprint == nil {
		return false
	}
	container := prog.GlobalVariablesBlueprint.Container()
	return container != nil && container.GetFunc() == fn
}

func clearFunctionBodyScopes(prog *Program, fn *Function) {
	if fn == nil {
		return
	}
	// A kept parameter points at its variable, and that variable points at the
	// whole scope. Nil-ing the block field alone leaves the version maps
	// reachable for the rest of the compile. Detach the variables first.
	if fn.builder != nil && fn.builder.CurrentBlock != nil {
		detachScope(fn.builder.CurrentBlock.ScopeTable)
		fn.builder.CurrentBlock.ScopeTable = nil
	}
	if prog == nil || prog.Cache == nil || prog.Cache.instructions == nil {
		return
	}
	for _, id := range fn.Blocks {
		inst, ok := prog.Cache.instructions.residentInstruction(id)
		if !ok || utils.IsNil(inst) {
			continue
		}
		block, ok := ToBasicBlock(inst)
		if !ok || block == nil {
			continue
		}
		detachScope(block.ScopeTable)
		block.ScopeTable = nil
	}
}

func detachScope(scope ScopeIF) {
	current, ok := scope.(*ScopeInstance)
	if !ok || current == nil {
		return
	}
	for _, latest := range current.GetAllVariables() {
		if latest == nil {
			continue
		}
		for _, versioned := range current.GetAllVariablesByName(latest.GetName(), true) {
			if versioned != nil {
				versioned.SetScope(nil)
			}
		}
	}
}

func releaseScopeOrdinaryValues(prog *Program, scope ScopeIF, visited map[*ScopeInstance]struct{}, reload func(int64) (Value, bool)) int {
	released := 0
	for scope != nil {
		current, ok := scope.(*ScopeInstance)
		if !ok || current == nil {
			return released
		}
		if _, seen := visited[current]; seen {
			return released
		}
		visited[current] = struct{}{}
		for _, latest := range current.GetAllVariables() {
			// GetAllVariables returns one head per name. The version chain also
			// owns earlier variables, so detaching just the head leaves all of
			// their ordinary values reachable through linkValues.
			for _, versioned := range current.GetAllVariablesByName(latest.GetName(), true) {
				variable, ok := versioned.(*Variable)
				if !ok || variable == nil || variable.Versioned == nil {
					continue
				}
				value := variable.Value
				if utils.IsNil(value) || valueCapturedByClosure(value) {
					continue
				}
				if shouldKeepCompileUnitBoundaryResident(value) {
					continue
				}
				id := value.GetId()
				if id <= 0 || !prog.Cache.instructions.isPersisted(id) {
					continue
				}
				current.UnlinkValue(value)
				variable.DetachRetained(id, reload)
				switch {
				case prog.Cache.instructions.writer != nil:
					prog.Cache.instructions.writer.DeleteIfSettled(id)
				case prog.Cache.instructions.residentCache != nil:
					prog.Cache.instructions.residentCache.DeleteClean(id)
				}
				// The head stays in linkValues. Older ordinary versions are not a
				// lexical lookup key once their saved id is on the variable itself.
				if versioned != latest {
					current.TrimSettledVersion(versioned)
				}
				released++
			}
		}
		scope = current.GetParent()
	}
	return released
}
