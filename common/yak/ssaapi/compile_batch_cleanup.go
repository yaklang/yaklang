package ssaapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// compileBatchCleanup owns the post-build work for one completed batch. It is
// a barrier before the next batch touches shared Program and cache state.
type compileBatchCleanup struct {
	config         *Config
	program        *ssa.Program
	apiProgram     *Program
	batch          compileUnitExecutionBatch
	unitKeys       []string
	structScan     bool
	incremental    bool
	batchIndex     int
	batchCount     int
	buildStarted   time.Time
	loadProgress   func() float64
	reportProgress func(float64, string, ...any)
}

func (task compileBatchCleanup) run() error {
	if task.structScan {
		index := &structResidentOpcodeIndex{prog: task.program}
		for _, unit := range task.batch.units {
			if unit == nil {
				continue
			}
			task.reportProgress(task.loadProgress(), fmt.Sprintf("[struct_scan] package=%s", unit.Key))
			task.config.structScan.scanStructWithOpcodeIndex(task.apiProgram, unit, index)
			if task.config.isStop() {
				return ErrContextCancel
			}
		}
		task.config.structScan.WaitSaves()
		if task.program.Cache != nil {
			task.program.Cache.EnableInstructionSpill()
		}
	}

	// The cache owns database selection. A batch may hand off persistence only
	// after its unit bodies have finished building.
	if task.program.Cache != nil {
		// One barrier line per batch. Without it a slow scan cannot be
		// attributed between building, spilling, auxiliary stores and
		// release: the per-batch wall clock alone does not say which step
		// held the compile thread.
		var barrierStart = time.Now()
		stepStart := barrierStart
		step := func() time.Duration {
			d := time.Since(stepStart)
			stepStart = time.Now()
			return d
		}
		// Nil finished body scopes before the flush evicts their blocks.
		// Otherwise the block leaves the resident map while Parameter.block
		// still owns the scope, and the version maps stay until the process ends.
		task.program.DetachFinishedFunctionBodyScopes()
		if !task.incremental {
			task.program.Cache.FlushCompileUnit(strings.Join(task.unitKeys, ","))
		}
		flushUnit := step()
		task.program.Cache.FlushInstructionSaver()
		flushInst := step()
		// Unlink ordinary scope values as soon as the instruction saver has
		// settled, before FlushAuxSavers can trigger another round of evictions.
		// FlushCompileUnit also evicts finished private BasicBlocks; waiting
		// until after FlushAuxSavers meant those blocks were already gone from
		// the resident map, so their scopes were never visited and stayed
		// reachable through Function.builder.CurrentBlock.
		task.program.ReleasePersistedOrdinaryValues()
		task.program.Cache.FlushAuxSavers()
		// Type/index serialization can reload instructions and enqueue their
		// eviction again; settle those writes before the final scope pass.
		flushAux := step()
		task.program.Cache.FlushInstructionSaver()
		flushInst2 := step()
		task.program.ReleasePersistedOrdinaryValues()
		releaseOrdinary := step()
		task.program.ReleaseSettledIndexes()
		releaseIndex := step()
		task.program.CheckMemoryPressure(task.batchIndex+1, task.batchCount)
		memPressure := step()
		if stats := task.program.Cache.InstructionCacheStats(); true {
			log.Infof("[compile-barrier] batch=%d/%d units=%d flush_unit=%s flush_inst=%s flush_aux=%s flush_inst2=%s release_ordinary=%s release_index=%s mem_pressure=%s total=%s resident=%d saver{%s}",
				task.batchIndex+1, task.batchCount, len(task.batch.units),
				flushUnit, flushInst, flushAux, flushInst2, releaseOrdinary, releaseIndex, memPressure,
				time.Since(barrierStart), stats.ResidentCount, stats.Saver)
		}
	}
	if compileUnitLogEnabled() {
		task.program.ProcessInfof("compile unit batch(%d/%d) build finished units=%s cost=%v",
			task.batchIndex+1, task.batchCount, strings.Join(task.unitKeys, ","), time.Since(task.buildStarted))
	}
	logPhaseHeap(fmt.Sprintf("unit_batch_%03d", task.batchIndex+1))
	return nil
}
