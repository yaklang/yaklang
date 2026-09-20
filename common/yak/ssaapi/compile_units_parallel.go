package ssaapi

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// compileUnitBuildWaves retains SCCs as indivisible work items. A size-bounded
// execution batch may contain dependent SCCs; only the ready SCCs in each wave
// can build concurrently. SCC fragments split across memory batches stay serial.
func compileUnitBuildWaves(plan *UnitPlan, batch compileUnitExecutionBatch) [][][]string {
	selected := make(map[string]bool, len(batch.unitKeys))
	for _, key := range batch.unitKeys {
		selected[key] = true
	}
	groups := make([][]string, 0)
	owner := make(map[string]int)
	for _, scc := range plan.Order {
		var keys []string
		for _, unit := range scc {
			if unit != nil && selected[unit.Key] {
				owner[unit.Key] = len(groups)
				keys = append(keys, unit.Key)
			}
		}
		if len(keys) > 0 {
			groups = append(groups, keys)
		}
	}
	deps := make([]map[int]bool, len(groups))
	for _, edge := range plan.Edges {
		from, fromOK := owner[edge.From]
		to, toOK := owner[edge.To]
		if fromOK && toOK && from != to {
			if deps[from] == nil {
				deps[from] = make(map[int]bool)
			}
			deps[from][to] = true
		}
	}
	done := make([]bool, len(groups))
	var waves [][][]string
	for remaining := len(groups); remaining > 0; {
		var ready []int
		for i := range groups {
			if done[i] {
				continue
			}
			blocked := false
			for dep := range deps[i] {
				if !done[dep] {
					blocked = true
					break
				}
			}
			if !blocked {
				ready = append(ready, i)
			}
		}
		// A valid condensation graph is acyclic. Keep malformed plans serial
		// rather than spinning or silently omitting their work.
		if len(ready) == 0 {
			for i := range groups {
				if !done[i] {
					waves = append(waves, [][]string{groups[i]})
				}
			}
			break
		}
		wave := make([][]string, 0, len(ready))
		for _, i := range ready {
			done[i] = true
			remaining--
			wave = append(wave, groups[i])
		}
		waves = append(waves, wave)
	}
	return waves
}

// buildCompileUnitWave never overlaps work owned by the application or the
// same library. Directory-based frontends can share an application builder
// even when the dependency graph says their file units are independent.
func buildCompileUnitWave(ctx context.Context, prog *ssa.Program, wave [][]string, concurrency int) (int, error) {
	return buildCompileUnitWaveWithOwners(ctx, prog, wave, concurrency, make(map[*ssa.Program]bool))
}

func buildCompileUnitWaveWithOwners(ctx context.Context, prog *ssa.Program, wave [][]string, concurrency int, built map[*ssa.Program]bool) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	var active, peak atomic.Int32
	slots := make(chan struct{}, concurrency)
	owners := make(map[*ssa.Program]bool)
	build := func(libraries []*ssa.Program) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old; old = peak.Load() {
			if peak.CompareAndSwap(old, current) {
				break
			}
		}
		for _, library := range libraries {
			library.LazyBuildCurrentProgram()
		}
	}
	for _, keys := range wave {
		if err := ctx.Err(); err != nil {
			wg.Wait()
			return int(peak.Load()), err
		}
		exclusive := false
		var libraries []*ssa.Program
		for _, key := range keys {
			lib := prog.ProgramForCompileUnit(key)
			if lib == nil || built[lib] {
				continue
			}
			built[lib] = true
			if lib == prog.GetApplication() || owners[lib] {
				exclusive = true
			}
			libraries = append(libraries, lib)
		}
		if len(libraries) == 0 {
			continue
		}
		if exclusive {
			wg.Wait()
			if err := ctx.Err(); err != nil {
				return int(peak.Load()), err
			}
			build(libraries)
			clear(owners)
			continue
		}
		for _, lib := range libraries {
			owners[lib] = true
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return int(peak.Load()), ctx.Err()
		}
		wg.Add(1)
		go func(libraries []*ssa.Program) {
			defer wg.Done()
			defer func() { <-slots }()
			if ctx.Err() == nil {
				build(libraries)
			}
		}(libraries)
	}
	wg.Wait()
	return int(peak.Load()), ctx.Err()
}
