package ssa

import "github.com/yaklang/yaklang/common/utils"

// ReleasePersistedOrdinaryValues drops strong scope references to ordinary
// instructions that have already been saved. The variable keeps the id and
// GetValue loads it again. Exported functions, externs, unfinished functions,
// closure-captured values, and boundary instructions stay as objects.
//
// Callers must run this only after the batch workers have joined and the
// instruction saver barrier has returned. It does not guess compile-unit
// names and does not clear fn.Blocks.
func (prog *Program) ReleasePersistedOrdinaryValues() int {
	if prog == nil || prog.Cache == nil || prog.Cache.instructions == nil {
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
		if fn == nil || !fn.finished || shouldKeepFunctionForCrossUnitReference(fn) {
			return
		}
		if fn.builder != nil && fn.builder.CurrentBlock != nil && fn.builder.CurrentBlock.ScopeTable != nil {
			released += releaseScopeOrdinaryValues(prog, fn.builder.CurrentBlock.ScopeTable, visitedScopes, reload)
		}
		for _, id := range fn.Blocks {
			block, ok := fn.GetBasicBlockByID(id)
			if !ok || block == nil || block.ScopeTable == nil {
				continue
			}
			released += releaseScopeOrdinaryValues(prog, block.ScopeTable, visitedScopes, reload)
		}
	})
	return released
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
		for _, versioned := range current.GetAllVariables() {
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
			if prog.Cache.instructions.writer != nil {
				prog.Cache.instructions.writer.DeleteIfSettled(id)
			}
			released++
		}
		scope = current.GetParent()
	}
	return released
}
