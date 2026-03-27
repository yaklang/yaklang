package ssa

import (
	"os"
	"time"

	"github.com/jinzhu/gorm"
	yaklog "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

type ProgramCacheKind int

const (
	ProgramCacheNone ProgramCacheKind = iota
	ProgramCacheMemory
	ProgramCacheDBRead
	ProgramCacheDBWrite
)

const (
	defaultSaveSize              = 200
	maxSaveSize                  = 40000
	saveTime                     = time.Second
	fastPathProjectByteThreshold = 2 * 1024 * 1024
	largeProjectByteThreshold    = 16 * 1024 * 1024
	largeProjectCacheTTL         = 250 * time.Millisecond
	largeProjectCacheMax         = 1024
	largeProjectInstructionSave  = 1024
	largeProjectPersistLimit     = 16384
	largeProjectAuxiliarySave    = 512
	largeProjectTypeSave         = 256
)

var hotInstructionOpcodeBlacklist = map[Opcode]struct{}{
	SSAOpcodeBasicBlock: {},
	SSAOpcodeFunction:   {},
	SSAOpcodeConstInst:  {},
	SSAOpcodeUndefined:  {},
	SSAOpcodeMake:       {},
}

func instructionCacheDebugEnabled() bool {
	return yaklog.GetLevel() >= yaklog.DebugLevel
}

func instructionCacheEventDebugEnabled() bool {
	return instructionCacheDebugEnabled() && os.Getenv("YAK_SSA_IR_CACHE_EVENT_DEBUG") != ""
}

func instructionReloadStackDebugEnabled() bool {
	return os.Getenv("YAK_SSA_IR_CACHE_RELOAD_STACK_DEBUG") != ""
}

func shouldKeepInstructionResident(inst Instruction) bool {
	if utils.IsNil(inst) {
		return false
	}
	_, ok := hotInstructionOpcodeBlacklist[inst.GetOpcode()]
	return ok
}

func shouldDelayInstructionEviction(inst Instruction) bool {
	if utils.IsNil(inst) {
		return true
	}
	fn := inst.GetFunc()
	if utils.IsNil(fn) {
		return true
	}
	return !fn.IsFinished()
}

func instructionLocationIDs(inst Instruction) (funcID, blockID int64) {
	if utils.IsNil(inst) {
		return 0, 0
	}
	if lz, ok := ToLazyInstruction(inst); ok && lz != nil && lz.ir != nil {
		return lz.ir.CurrentFunction, lz.ir.CurrentBlock
	}
	if inner := inst.getAnInstruction(); inner != nil {
		return inner.funcId, inner.blockId
	}
	return 0, 0
}

type cacheBackend[T dbcache.MemoryItem] interface {
	Set(T)
	Get(int64) (T, bool)
	Delete(int64)
	CoolDown([]int64, time.Duration)
	Track([]int64)
	Count() int
	ForEach(func(int64, T) bool)
	GetAll() map[int64]T
	Stats() dbcache.CacheStats
	Close()
}

type Cache[T dbcache.MemoryItem] struct {
	backend cacheBackend[T]
	id      *atomic.Int64
}

func NewCache[T dbcache.MemoryItem](backend cacheBackend[T]) *Cache[T] {
	if backend == nil {
		backend = &memoryCacheBackend[T]{
			SafeMapWithKey: utils.NewSafeMapWithKey[int64, T](),
		}
	}
	return &Cache[T]{
		backend: backend,
		id:      atomic.NewInt64(0),
	}
}

func (c *Cache[T]) Set(item T) {
	if c == nil || utils.IsNil(item) {
		return
	}
	id := item.GetId()
	if id <= 0 {
		id = c.id.Inc()
		item.SetId(id)
	}
	c.backend.Set(item)
}

func (c *Cache[T]) Get(id int64) (T, bool) {
	if c == nil || c.backend == nil {
		return *new(T), false
	}
	return c.backend.Get(id)
}

func (c *Cache[T]) Delete(id int64) {
	if c == nil || c.backend == nil {
		return
	}
	c.backend.Delete(id)
}

func (c *Cache[T]) Count() int {
	if c == nil || c.backend == nil {
		return 0
	}
	return c.backend.Count()
}

func (c *Cache[T]) CoolDown(ids []int64, ttl time.Duration) {
	if c == nil || c.backend == nil || len(ids) == 0 || ttl <= 0 {
		return
	}
	c.backend.CoolDown(ids, ttl)
}

func (c *Cache[T]) Track(ids []int64) {
	if c == nil || c.backend == nil || len(ids) == 0 {
		return
	}
	c.backend.Track(ids)
}

func (c *Cache[T]) ForEach(f func(int64, T) bool) {
	if c == nil || c.backend == nil {
		return
	}
	c.backend.ForEach(f)
}

func (c *Cache[T]) GetAll() map[int64]T {
	if c == nil || c.backend == nil {
		return nil
	}
	return c.backend.GetAll()
}

func (c *Cache[T]) Stats() dbcache.CacheStats {
	if c == nil || c.backend == nil {
		return dbcache.CacheStats{}
	}
	return c.backend.Stats()
}

func (c *Cache[T]) Close() {
	if c == nil || c.backend == nil {
		return
	}
	c.backend.Close()
}

type spillToggle interface {
	EnableSave()
	DisableSave()
	IsSaveDisabled() bool
}

func (c *Cache[T]) EnableSave() {
	if c == nil || c.backend == nil {
		return
	}
	if toggle, ok := c.backend.(spillToggle); ok {
		toggle.EnableSave()
	}
}

func (c *Cache[T]) DisableSave() {
	if c == nil || c.backend == nil {
		return
	}
	if toggle, ok := c.backend.(spillToggle); ok {
		toggle.DisableSave()
	}
}

func (c *Cache[T]) IsSaveDisabled() bool {
	if c == nil || c.backend == nil {
		return false
	}
	if toggle, ok := c.backend.(spillToggle); ok {
		return toggle.IsSaveDisabled()
	}
	return false
}

type memoryCacheBackend[T dbcache.MemoryItem] struct {
	*utils.SafeMapWithKey[int64, T]
}

var _ cacheBackend[Instruction] = (*memoryCacheBackend[Instruction])(nil)
var _ cacheBackend[Type] = (*memoryCacheBackend[Type])(nil)

func (b *memoryCacheBackend[T]) Set(item T) {
	b.SafeMapWithKey.Set(item.GetId(), item)
}

func (b *memoryCacheBackend[T]) CoolDown(_ []int64, _ time.Duration) {}

func (b *memoryCacheBackend[T]) Track(_ []int64) {}

func (b *memoryCacheBackend[T]) Stats() dbcache.CacheStats { return dbcache.CacheStats{} }

func (b *memoryCacheBackend[T]) Close() {}

type dbcacheBackend[T dbcache.MemoryItem, D any] struct {
	*dbcache.Cache[T, D]
}

var _ cacheBackend[Instruction] = (*dbcacheBackend[Instruction, *instructionPersistRecord])(nil)
var _ cacheBackend[Type] = (*dbcacheBackend[Type, *ssadb.IrType])(nil)
var _ spillToggle = (*dbcacheBackend[Instruction, *instructionPersistRecord])(nil)
var _ spillToggle = (*dbcacheBackend[Type, *ssadb.IrType])(nil)

func (b *dbcacheBackend[T, D]) Track(ids []int64) {
	if b == nil || b.Cache == nil {
		return
	}
	b.Cache.Track(ids)
}

type serializingPersistence[T dbcache.MemoryItem, D any] struct {
	pipe  *pipeline.Pipe[T, *struct{}]
	saver *dbcache.Save[D]
}

func newSerializingPersistence[T dbcache.MemoryItem, D any](
	cfg *ssaconfig.Config,
	saveSize int,
	name string,
	saveParallelism int,
	marshal dbcache.MarshalFunc[T, D],
	save dbcache.SaveFunc[D],
) *serializingPersistence[T, D] {
	cfg = ensureProgramConfig(cfg)
	saver := dbcache.NewSave(func(items []D) {
		if save == nil {
			return
		}
		if err := save(items); err != nil {
			log.Errorf("dbcache serializing save failed: %v", err)
		}
	},
		dbcache.WithContext(cfg.GetContext()),
		dbcache.WithSaveSize(saveSize),
		dbcache.WithSaveTimeout(saveTime),
		dbcache.WithSaveParallelism(saveParallelism),
		dbcache.WithName(name),
	)
	pipe := pipeline.NewPipe(cfg.GetContext(), saveSize, func(item T) (*struct{}, error) {
		if marshal == nil {
			return nil, nil
		}
		data, err := marshal(item, utils.EvictionReasonDeleted)
		if err != nil {
			return nil, err
		}
		if utils.IsNil(data) {
			return nil, nil
		}
		saver.Save(data)
		return nil, nil
	})
	return &serializingPersistence[T, D]{
		pipe:  pipe,
		saver: saver,
	}
}

func (p *serializingPersistence[T, D]) Save(item T) {
	if p == nil || utils.IsNil(item) {
		return
	}
	p.pipe.Feed(item)
}

func (p *serializingPersistence[T, D]) Close() {
	if p == nil {
		return
	}
	p.pipe.Close()
	p.saver.Close()
}

func (p *serializingPersistence[T, D]) Stats() dbcache.CacheStats {
	if p == nil || p.saver == nil {
		return dbcache.CacheStats{}
	}
	return dbcache.CacheStats{Saver: p.saver.Stats()}
}

type serializingCacheBackend[T dbcache.MemoryItem, D any] struct {
	*utils.SafeMapWithKey[int64, T]
	persistence *serializingPersistence[T, D]
}

var _ cacheBackend[Instruction] = (*serializingCacheBackend[Instruction, *instructionPersistRecord])(nil)
var _ cacheBackend[Type] = (*serializingCacheBackend[Type, *ssadb.IrType])(nil)

func (b *serializingCacheBackend[T, D]) Set(item T) {
	b.SafeMapWithKey.Set(item.GetId(), item)
}

func (b *serializingCacheBackend[T, D]) CoolDown(_ []int64, _ time.Duration) {}

func (b *serializingCacheBackend[T, D]) Track(_ []int64) {}

func (b *serializingCacheBackend[T, D]) Stats() dbcache.CacheStats {
	if b == nil {
		return dbcache.CacheStats{}
	}
	stats := dbcache.CacheStats{}
	if b.SafeMapWithKey != nil {
		stats.ResidentCount = b.SafeMapWithKey.Count()
	}
	if b.persistence != nil {
		stats.Saver = b.persistence.Stats().Saver
	}
	return stats
}

func (b *serializingCacheBackend[T, D]) Close() {
	if b == nil || b.persistence == nil {
		return
	}
	b.SafeMapWithKey.ForEach(func(_ int64, item T) bool {
		b.persistence.Save(item)
		return true
	})
	b.persistence.Close()
}

func useAdaptiveInstructionFastPath(cfg *ssaconfig.Config) bool {
	cfg = ensureProgramConfig(cfg)
	ttl, maxEntries := resolveInstructionCacheSettings(cfg)
	return ttl == 0 &&
		maxEntries == 0 &&
		cfg.GetCompileProjectBytes() > 0 &&
		cfg.GetCompileProjectBytes() <= fastPathProjectByteThreshold
}

func isLargeProjectCompile(cfg *ssaconfig.Config) bool {
	cfg = ensureProgramConfig(cfg)
	return cfg.GetCompileProjectBytes() >= largeProjectByteThreshold
}

func resolveInstructionPersistenceTuning(cfg *ssaconfig.Config, saveSize int) (int, int) {
	if isLargeProjectCompile(cfg) {
		return largeProjectInstructionSave, largeProjectPersistLimit
	}
	return saveSize, 0
}

func resolveAuxiliarySaveSize(cfg *ssaconfig.Config, saveSize int) int {
	if isLargeProjectCompile(cfg) {
		return largeProjectAuxiliarySave
	}
	if saveSize < IndexSaveSize {
		return IndexSaveSize
	}
	return saveSize
}

func resolveTypeSaveSize(cfg *ssaconfig.Config, saveSize int) int {
	if isLargeProjectCompile(cfg) {
		return largeProjectTypeSave
	}
	return min(max(saveSize*10, 2000), maxSaveSize)
}

func resolveInstructionCacheSettings(cfg *ssaconfig.Config) (time.Duration, int) {
	cfg = ensureProgramConfig(cfg)
	ttl := cfg.GetCompileIrCacheTTL()
	maxEntries := cfg.GetCompileIrCacheMax()
	if ttl == time.Second && maxEntries == 5000 && cfg.GetCompileProjectBytes() > 0 && cfg.GetCompileProjectBytes() <= fastPathProjectByteThreshold {
		return 0, 0
	}
	if ttl == time.Second && maxEntries == 5000 && cfg.GetCompileProjectBytes() >= largeProjectByteThreshold {
		return largeProjectCacheTTL, largeProjectCacheMax
	}
	return ttl, maxEntries
}

type instructionPersistRecord struct {
	IrCode    *ssadb.IrCode
	Opcode    Opcode
	Reason    utils.EvictionReason
	Writeback bool
	Editor    *memedit.MemEditor
	CodeID    int64
}

func saveInstructionIrCodesFast(prog *Program, db *gorm.DB, f func(int)) dbcache.SaveFunc[*ssadb.IrCode] {
	return func(records []*ssadb.IrCode) error {
		if len(records) == 0 {
			return nil
		}
		var saveErr error
		saveStep := func() error {
			saveErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irCode := range records {
					if irCode == nil {
						continue
					}
					if err := tx.Save(irCode).Error; err != nil {
						return err
					}
				}
				return nil
			})
			return saveErr
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrCodeBatchFastPath", saveStep)
		} else {
			saveStep()
		}
		if saveErr != nil {
			return saveErr
		}
		if f != nil {
			f(len(records))
		}
		return nil
	}
}

func createInstructionCache(
	cfg *ssaconfig.Config,
	databaseKind ProgramCacheKind,
	db *gorm.DB,
	prog *Program,
	saveSize int,
	saveFinish func(int),
) *Cache[Instruction] {
	cfg = ensureProgramConfig(cfg)
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	if databaseKind == ProgramCacheMemory || prog == nil {
		return NewCache[Instruction](&memoryCacheBackend[Instruction]{
			SafeMapWithKey: utils.NewSafeMapWithKey[int64, Instruction](),
		})
	}

	var (
		marshal dbcache.MarshalFunc[Instruction, *instructionPersistRecord]
		save    dbcache.SaveFunc[*instructionPersistRecord]
	)

	if databaseKind == ProgramCacheDBWrite && db != nil {
		marshal = createInstructionCacheMarshalFunc(prog)
		save = saveInstructionPersistRecords(prog, db, saveFinish)
	}

	if databaseKind == ProgramCacheDBWrite && db != nil && useAdaptiveInstructionFastPath(cfg) {
		fastSaveSize := min(max(saveSize*20, 5000), maxSaveSize)
		backend := &serializingCacheBackend[Instruction, *ssadb.IrCode]{
			SafeMapWithKey: utils.NewSafeMapWithKey[int64, Instruction](),
			persistence: newSerializingPersistence(
				cfg,
				fastSaveSize,
				"InstructionClose",
				8,
				func(inst Instruction, _ utils.EvictionReason) (*ssadb.IrCode, error) {
					return marshalIrCode(inst)
				},
				saveInstructionIrCodesFast(prog, db, saveFinish),
			),
		}
		return NewCache[Instruction](backend)
	}

	cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
	instructionSaveSize, persistLimit := resolveInstructionPersistenceTuning(cfg, saveSize)
	backend := dbcache.NewCache[Instruction, *instructionPersistRecord](
		cacheTTL,
		cacheMax,
		marshal,
		save,
		createInstructionCacheLoadFunc(prog),
		dbcache.WithContext(cfg.GetContext()),
		dbcache.WithSaveSize(instructionSaveSize),
		dbcache.WithPersistLimit(persistLimit),
		dbcache.WithSaveTimeout(saveTime),
		dbcache.WithName("Instruction"),
		dbcache.WithSkipEviction(func(inst Instruction) bool {
			return shouldKeepInstructionResident(inst) || shouldDelayInstructionEviction(inst)
		}),
	)
	return NewCache[Instruction](&dbcacheBackend[Instruction, *instructionPersistRecord]{Cache: backend})
}

func createTypeCache(
	cfg *ssaconfig.Config,
	db *gorm.DB,
	prog *Program,
	programName string,
	saveSize int,
) *Cache[Type] {
	cfg = ensureProgramConfig(cfg)
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	if prog == nil || db == nil {
		return NewCache[Type](&memoryCacheBackend[Type]{
			SafeMapWithKey: utils.NewSafeMapWithKey[int64, Type](),
		})
	}

	serializingBackend := &serializingCacheBackend[Type, *ssadb.IrType]{
		SafeMapWithKey: utils.NewSafeMapWithKey[int64, Type](),
		persistence: newSerializingPersistence(
			cfg,
			resolveTypeSaveSize(cfg, saveSize),
			"Type",
			8,
			marshalIrType(programName),
			saveIrType(prog, db),
		),
	}
	return NewCache[Type](serializingBackend)

}

func marshalIrCode(s Instruction) (*ssadb.IrCode, error) {
	ret := ssadb.EmptyIrCode(s.GetProgramName(), s.GetId())
	if ok := marshalInstruction(s, ret); !ok {
		return nil, nil
	}
	return ret, nil
}

func createInstructionCacheMarshalFunc(prog *Program) dbcache.MarshalFunc[Instruction, *instructionPersistRecord] {
	return func(inst Instruction, reason utils.EvictionReason) (*instructionPersistRecord, error) {
		irCode, err := marshalIrCode(inst)
		if err != nil {
			return nil, err
		}

		writeback := false
		if lz, ok := ToLazyInstruction(inst); ok && lz != nil {
			writeback = lz.ShouldSave()
		}

		if irCode == nil {
			if instructionCacheEventDebugEnabled() {
				log.Debugf("[ssa-ir-cache] save-skip: program=%s id=%d opcode=%s reason=%s",
					prog.GetProgramName(),
					inst.GetId(),
					inst.GetOpcode().String(),
					evictionReasonName(reason),
				)
			}
			if prog.Cache != nil && prog.Cache.instructionCacheMetrics != nil {
				prog.Cache.instructionCacheMetrics.RecordEvict(reason, inst.GetOpcode())
			}
			return nil, nil
		}

		return &instructionPersistRecord{
			IrCode:    irCode,
			Opcode:    inst.GetOpcode(),
			Reason:    reason,
			Writeback: writeback,
			Editor:    instructionEditor(inst),
			CodeID:    inst.GetId(),
		}, nil
	}
}

func marshalIrType(name string) dbcache.MarshalFunc[Type, *ssadb.IrType] {
	return func(s Type, _ utils.EvictionReason) (*ssadb.IrType, error) {
		if s.GetId() <= 0 {
			log.Errorf("[BUG] marshalIrType: type ID is invalid: %d, type: %s", s.GetId(), s.String())
		}

		ret := ssadb.EmptyIrType(name, uint64(s.GetId()))
		marshalType(s, ret)
		return ret, nil
	}
}

func saveInstructionPersistRecords(prog *Program, db *gorm.DB, f func(int)) dbcache.SaveFunc[*instructionPersistRecord] {
	return func(records []*instructionPersistRecord) error {
		if len(records) == 0 {
			return nil
		}

		start := time.Now()
		sourceAttempts := 0
		sources := make(map[string]*ssadb.IrSource)
		var saveErr error
		saveStep := func() error {
			saveErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, record := range records {
					if record == nil || record.IrCode == nil {
						continue
					}
					if record.IrCode.SourceCodeHash != "" && record.Editor != nil {
						sourceAttempts++
						if prog != nil && prog.Cache != nil && prog.Cache.IsExistedSourceCodeHash(prog.GetProgramName(), record.IrCode.SourceCodeHash) {
							continue
						}
						if _, ok := sources[record.IrCode.SourceCodeHash]; !ok {
							sources[record.IrCode.SourceCodeHash] = ssadb.MarshalFile(record.Editor, record.IrCode.SourceCodeHash)
						}
					}
				}
				for _, source := range sources {
					if err := tx.Save(source).Error; err != nil {
						return err
					}
				}
				for _, record := range records {
					if record == nil || record.IrCode == nil {
						continue
					}
					if record.Writeback {
						if err := ssadb.UpsertIrCode(tx, record.IrCode); err != nil {
							return err
						}
						continue
					}
					if err := tx.Save(record.IrCode).Error; err != nil {
						return err
					}
				}
				return nil
			})
			return saveErr
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrCodeBatch", saveStep)
		} else {
			saveStep()
		}
		cost := time.Since(start)
		perItemCost := cost / time.Duration(len(records))
		if perItemCost <= 0 {
			perItemCost = cost
		}

		if saveErr != nil {
			return saveErr
		}
		if f != nil {
			f(len(records))
		}
		if prog != nil && prog.Cache != nil && prog.Cache.instructionCacheMetrics != nil {
			prog.Cache.instructionCacheMetrics.RecordSourceSave(sourceAttempts, len(sources))
		}

		for _, record := range records {
			if record == nil {
				continue
			}
			if prog != nil && prog.Cache != nil && prog.Cache.instructionCacheMetrics != nil {
				prog.Cache.instructionCacheMetrics.RecordEvict(record.Reason, record.Opcode)
				if record.Writeback {
					prog.Cache.instructionCacheMetrics.RecordWriteback(record.Reason, record.Opcode, perItemCost)
				} else {
					prog.Cache.instructionCacheMetrics.RecordSave(record.Reason, record.Opcode, perItemCost)
				}
			}
			if record.Writeback {
				if instructionCacheEventDebugEnabled() {
					log.Debugf("[ssa-ir-cache] writeback: program=%s id=%d opcode=%s reason=%s cost=%s",
						prog.GetProgramName(), record.CodeID, record.Opcode.String(), evictionReasonName(record.Reason), perItemCost,
					)
				}
			} else {
				if instructionCacheEventDebugEnabled() {
					log.Debugf("[ssa-ir-cache] save: program=%s id=%d opcode=%s reason=%s cost=%s",
						prog.GetProgramName(), record.CodeID, record.Opcode.String(), evictionReasonName(record.Reason), perItemCost,
					)
				}
			}
		}
		return nil
	}
}

func saveIrType(prog *Program, db *gorm.DB) dbcache.SaveFunc[*ssadb.IrType] {
	return func(types []*ssadb.IrType) error {
		var saveErr error
		saveStep := func() error {
			saveErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irType := range types {
					if irType == nil {
						continue
					}
					if err := tx.Save(irType).Error; err != nil {
						return err
					}
				}
				return nil
			})
			return saveErr
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrTypeBatch", saveStep)
		} else {
			saveStep()
		}
		return saveErr
	}
}

func createInstructionCacheLoadFunc(prog *Program) dbcache.LoadFunc[Instruction] {
	return func(id int64) (Instruction, error) {
		start := time.Now()
		inst, err := NewLazyInstruction(prog, id)
		if err != nil {
			return nil, err
		}
		if prog != nil && prog.Cache != nil && prog.Cache.instructionCacheMetrics != nil {
			prog.Cache.instructionCacheMetrics.RecordReload(inst.GetOpcode(), time.Since(start))
		}
		if instructionCacheEventDebugEnabled() {
			log.Debugf("[ssa-ir-cache] reload: program=%s id=%d opcode=%s cost=%s",
				prog.GetProgramName(), id, inst.GetOpcode().String(), time.Since(start),
			)
		}
		if instructionReloadStackDebugEnabled() {
			log.Warnf("[ssa-ir-cache-reload] program=%s id=%d opcode=%s cost=%s",
				prog.GetProgramName(), id, inst.GetOpcode().String(), time.Since(start),
			)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		return inst, nil
	}
}

func instructionEditor(inst Instruction) *memedit.MemEditor {
	if inst == nil {
		return nil
	}
	if r := inst.GetRange(); r != nil {
		if editor := r.GetEditor(); editor != nil {
			return editor
		}
	}
	if block := inst.GetBlock(); block != nil && block.GetRange() != nil {
		if editor := block.GetRange().GetEditor(); editor != nil {
			return editor
		}
	}
	if fn := inst.GetFunc(); fn != nil && fn.GetRange() != nil {
		if editor := fn.GetRange().GetEditor(); editor != nil {
			return editor
		}
	}
	return nil
}
