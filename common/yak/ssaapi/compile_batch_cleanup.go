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
		if !task.incremental {
			task.program.Cache.FlushCompileUnit(strings.Join(task.unitKeys, ","))
		}
		task.program.Cache.FlushInstructionSaver()
		task.program.Cache.FlushAuxSavers()
		// Workers for this batch have joined, and both saver barriers have
		// returned. Only then may ordinary persisted values leave the scopes.
		task.program.ReleasePersistedOrdinaryValues()
		task.program.CheckMemoryPressure(task.batchIndex+1, task.batchCount)
	}
	if compileUnitLogEnabled() {
		task.program.ProcessInfof("compile unit batch(%d/%d) build finished units=%s cost=%v",
			task.batchIndex+1, task.batchCount, strings.Join(task.unitKeys, ","), time.Since(task.buildStarted))
	}
	logPhaseHeap(fmt.Sprintf("unit_batch_%03d", task.batchIndex+1))
	return nil
}
