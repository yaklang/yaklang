package ssa

import (
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

type ProgramCacheKind int

const (
	_ ProgramCacheKind = iota
	ProgramCacheMemory
	ProgramCacheDBRead
	ProgramCacheDBWrite
)

type ProgramCache struct {
	program *Program
	db      *gorm.DB

	instructions *instructionStore
	types        *typeStore
	sources      *sourceStore
	indexes      *indexStore

	instructionMetrics *instructionCacheMetrics
}

func NewDBCache(cfg *ssaconfig.Config, prog *Program, databaseKind ProgramCacheKind, fileSize int) *ProgramCache {
	cfg = ensureProgramConfig(cfg)
	cache := &ProgramCache{
		program: prog,
	}

	var programName string
	if databaseKind != ProgramCacheMemory {
		programName = prog.GetApplication().GetProgramName()
		cache.db = ssadb.GetDB().Where("program_name = ?", programName)
	}
	if databaseKind != ProgramCacheMemory && instructionCacheDebugEnabled() {
		cache.instructionMetrics = newInstructionCacheMetrics()
		cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
		log.Debugf("[ssa-ir-cache] init: program=%s ttl=%s max=%d kind=%d",
			programName, cacheTTL, cacheMax, databaseKind,
		)
	}

	saveSize := min(max(fileSize*5, defaultSaveSize), maxSaveSize)
	log.Debugf("asyncdb Channel: ReSetSize: fileSize(%d) saveSize(%d)", fileSize, saveSize)

	cache.sources = newSourceStore(prog, databaseKind, cache.db)
	cache.indexes = newIndexStore(cfg, prog, databaseKind, cache.db, saveSize/2)
	cache.types = newTypeStore(cfg, prog, databaseKind, cache.db, programName, saveSize)
	cache.instructions = newInstructionStore(cfg, prog, databaseKind, cache.db, saveSize, cache.sources, cache.instructionMetrics)
	return cache
}

func (c *ProgramCache) HaveDatabaseBackend() bool {
	return c != nil && c.db != nil
}

func (c *ProgramCache) DebugDB() {
	if c == nil || c.db == nil {
		return
	}
	c.db = c.db.Debug()
}

func (c *ProgramCache) DisableInstructionSpill() {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return
	}
	c.instructions.DisableSpill()
}

func (c *ProgramCache) EnableInstructionSpill() {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return
	}
	c.instructions.EnableSpill()
}

func (c *ProgramCache) InstructionSpillDisabled() bool {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return false
	}
	return c.instructions.SpillDisabled()
}

func (c *ProgramCache) SetInstruction(inst Instruction) {
	if utils.IsNil(inst) {
		log.Errorf("BUG: SetInstruction called with nil instruction")
		return
	}
	if c != nil && c.indexes != nil {
		c.indexes.AddInstructionOffsets(inst)
	}
	if c != nil && c.instructions != nil {
		c.instructions.Set(inst)
	}
}

func (c *ProgramCache) DeleteInstruction(inst Instruction) {
	if c == nil || c.instructions == nil || utils.IsNil(inst) {
		return
	}
	c.instructions.Delete(inst.GetId())
}

func (c *ProgramCache) GetInstruction(id int64) Instruction {
	if c == nil || c.instructions == nil || id == 0 {
		return nil
	}
	return c.instructions.Get(id)
}

func (c *ProgramCache) PreloadInstructionsByIDsFast(ids []int64) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.PreloadByIDsFast(ids)
}

func (c *ProgramCache) AddConst(inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddConst(inst)
}

func (c *ProgramCache) AddVariable(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddVariable(name, inst)
}

func (c *ProgramCache) RemoveVariable(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.RemoveVariable(name, inst)
}

func (c *ProgramCache) AddClassInstance(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddClassInstance(name, inst)
}

func (c *ProgramCache) SaveToDatabase(cb ...func(int)) {
	if !c.HaveDatabaseBackend() {
		return
	}
	progress := func(int) {}
	if len(cb) > 0 && cb[0] != nil {
		progress = cb[0]
	}

	steps := []func() error{
		func() error {
			if c.types != nil {
				c.types.close()
				log.Infof("Type Cache closed")
			}
			return nil
		},
		func() error {
			if c.indexes != nil {
				c.indexes.Close()
			}
			return nil
		},
		func() error {
			if c.instructions != nil {
				c.instructions.Close(progress)
				log.Infof("Instruction cache closed")
			}
			return nil
		},
		func() error {
			if c.sources != nil {
				c.sources.Close()
			}
			return nil
		},
		func() error {
			if c.program != nil && c.instructions != nil {
				stats := c.instructions.Stats()
				log.Debugf("[ssa-ir-cache-saver] program=%s resident=%d pending=%d pending_max=%d batch_count=%d avg_batch=%.2f max_batch=%d enqueue_block_total=%s enqueue_block_max=%s save_loop_time=%s save_loop_max=%s",
					c.program.GetProgramName(),
					stats.ResidentCount,
					stats.Saver.Pending,
					stats.Saver.MaxPending,
					stats.Saver.BatchCount,
					stats.Saver.AvgBatchSize(),
					stats.Saver.MaxBatchSize,
					stats.Saver.EnqueueBlockTotal,
					stats.Saver.MaxEnqueueBlock,
					stats.Saver.SaveTimeTotal,
					stats.Saver.MaxSaveTime,
				)
			}
			if c.instructionMetrics != nil && c.program != nil {
				c.instructionMetrics.Dump(c.program.GetProgramName())
			}
			return nil
		},
	}
	c.diagnosticsTrack("ssa.ProgramCache.SaveToDatabase", steps...)
}

func (c *ProgramCache) CountInstruction() int {
	if c == nil || c.instructions == nil {
		return 0
	}
	return c.instructions.Count()
}

func (c *ProgramCache) CoolDownFunctionInstructions(function *Function) {
	if c == nil || c.instructions == nil || !c.HaveDatabaseBackend() || c.program == nil || c.program.DatabaseKind != ProgramCacheDBWrite {
		return
	}
	c.instructions.TrackFunctionFinish(function)
}

func (c *ProgramCache) rememberType(typ Type) {
	if c == nil || c.types == nil || utils.IsNil(typ) {
		return
	}
	c.types.remember(typ)
}

func (c *ProgramCache) getType(id int64) (Type, bool) {
	if c == nil || c.types == nil {
		return nil, false
	}
	return c.types.get(id)
}

func (c *ProgramCache) residentType(id int64) (Type, bool) {
	if c == nil || c.types == nil || c.types.resident == nil {
		return nil, false
	}
	return c.types.resident.Get(id)
}

func (c *ProgramCache) coolDownInstructions(ids []int64, ttl time.Duration) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.CoolDown(ids, ttl)
}

func (c *ProgramCache) deleteInstructionByID(id int64) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.Delete(id)
}

func (c *ProgramCache) residentInstructions() map[int64]Instruction {
	if c == nil || c.instructions == nil {
		return nil
	}
	return c.instructions.GetAllResident()
}

func (c *ProgramCache) hasResidentInstruction(id int64) bool {
	if id <= 0 {
		return false
	}
	_, ok := c.residentInstructions()[id]
	return ok
}

func (c *ProgramCache) findByVariableEx(mod ssadb.MatchMode, checkValue func(string) bool) []Instruction {
	if c == nil || c.indexes == nil {
		return nil
	}
	return c.indexes.FindByVariableEx(mod, checkValue, c.GetInstruction)
}

func trackAtomicMax(counter *atomic.Int64, value int64) {
	if counter == nil {
		return
	}
	for {
		current := counter.Load()
		if value <= current {
			return
		}
		if counter.CAS(current, value) {
			return
		}
	}
}

func normalizeVariableName(name string) (normalized, member string) {
	if strings.HasPrefix(name, "#") {
		if _, memberName, ok := strings.Cut(name, "."); ok {
			member = memberName
		}
		if _, memberKey, ok := strings.Cut(name, "["); ok {
			member, _ = strings.CutSuffix(memberKey, "]")
		}
	}
	if len(name) > 1 {
		name = strings.TrimPrefix(name, "$")
	}
	return name, member
}
