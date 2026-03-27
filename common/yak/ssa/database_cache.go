package ssa

import (
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Cache : a cache in middle layer of database and application.
//
//	application will Get/Set Instruction,
//
// and save the data to database when the data is expired,
// and load the data from database when the data is not in cache.

type ProgramCache struct {
	program          *Program // mark which program handled
	ProgramCacheKind ProgramCacheKind
	DB               *gorm.DB

	InstructionCache *Cache[Instruction]
	TypeCache        *Cache[Type]

	VariableIndex *SimpleCache[int64]
	MemberIndex   *SimpleCache[int64]
	ClassIndex    *SimpleCache[int64]
	ConstCache    *SimpleCache[int64]

	indexCache      *SimpleCache[*ssadb.IrIndex]
	offsetCache     *SimpleCache[*ssadb.IrOffset]
	editorCache     *SimpleCache[*ssadb.IrSource]
	editorHashCache *utils.SafeMapWithKey[string, struct{}]

	afterSaveNotify func(int)

	waitGroup *sync.WaitGroup // wait for all goroutines to finish

	instructionCacheMetrics *instructionCacheMetrics
}

func NewDBCache(cfg *ssaconfig.Config, prog *Program, databaseKind ProgramCacheKind, fileSize int) *ProgramCache {
	cfg = ensureProgramConfig(cfg)
	cache := &ProgramCache{
		program:          prog,
		ProgramCacheKind: databaseKind,
		waitGroup:        &sync.WaitGroup{},
	}
	if databaseKind != ProgramCacheMemory && instructionCacheDebugEnabled() {
		cache.instructionCacheMetrics = newInstructionCacheMetrics()
	}
	var programName string
	if databaseKind != ProgramCacheMemory { // database write/read
		programName = prog.GetApplication().GetProgramName()
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
		if instructionCacheDebugEnabled() {
			cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
			log.Debugf("[ssa-ir-cache] init: program=%s ttl=%s max=%d kind=%d",
				programName, cacheTTL, cacheMax, databaseKind,
			)
		}
	}
	saveSize := min(max(fileSize*5, defaultSaveSize), maxSaveSize)
	log.Debugf("asyncdb Channel: ReSetSize: fileSize(%d) saveSize(%d)", fileSize, saveSize)
	cache.initIndex(cfg, databaseKind, saveSize/2)
	cache.afterSaveNotify = func(i int) {}
	cache.InstructionCache = createInstructionCache(
		cfg, databaseKind,
		cache.DB, prog,
		saveSize,
		func(size int) {
			cache.afterSaveNotify(size)
		},
	)
	cache.TypeCache = createTypeCache(
		cfg, cache.DB, prog,
		programName, saveSize,
	)
	return cache
}

func (c *ProgramCache) HaveDatabaseBackend() bool {
	return c.DB != nil
}

func (c *ProgramCache) DisableInstructionSpill() {
	if c == nil || c.InstructionCache == nil || !c.HaveDatabaseBackend() {
		return
	}
	c.InstructionCache.DisableSave()
}

func (c *ProgramCache) EnableInstructionSpill() {
	if c == nil || c.InstructionCache == nil || !c.HaveDatabaseBackend() {
		return
	}
	c.InstructionCache.EnableSave()
}

func (c *ProgramCache) InstructionSpillDisabled() bool {
	if c == nil || c.InstructionCache == nil || !c.HaveDatabaseBackend() {
		return false
	}
	return c.InstructionCache.IsSaveDisabled()
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *ProgramCache) SetInstruction(inst Instruction) {
	if utils.IsNil(inst) {
		log.Errorf("BUG: SetInstruction called with nil instruction")
		return
	}
	if !utils.IsNil(c.offsetCache) {
		c.offsetCache.Add("", ConvertValue2Offset(inst))
	}
	c.InstructionCache.Set(inst)
}

func (c *ProgramCache) DeleteInstruction(inst Instruction) {
	c.InstructionCache.Delete(inst.GetId())
}

// GetInstruction : get instruction from cache.
func (c *ProgramCache) GetInstruction(id int64) Instruction {
	if id == 0 {
		return nil
	}
	if ret, ok := c.InstructionCache.Get(id); ok {
		return ret
	}
	return nil

}

// PreloadInstructionsByIDsFast fills instruction cache with lazy instructions without neighbor prefetch.
func (c *ProgramCache) PreloadInstructionsByIDsFast(ids []int64) {
	if c == nil || c.ProgramCacheKind != ProgramCacheDBRead || c.program == nil {
		return
	}
	if len(ids) == 0 {
		return
	}
	ssadb.PreloadIrCodesByIdsFast(ssadb.GetDB(), c.program.Name, ids)
	cache := ssadb.GetIrCodeCache(c.program.Name)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := c.InstructionCache.Get(id); ok {
			continue
		}
		if ir, ok := cache.Get(id); ok {
			if inst, err := NewLazyInstructionFromIrCode(ir, c.program); err == nil {
				c.InstructionCache.Set(inst)
			}
		}
	}
}

// =============================================== Variable =======================================================

func (c *ProgramCache) AddConst(inst Instruction) {
	if c == nil || c.ConstCache == nil || utils.IsNil(inst) {
		return
	}
	c.ConstCache.Add(inst.GetName(), inst.GetId())
}

func (c *ProgramCache) AddVariable(name string, inst Instruction) {
	if c == nil || c.VariableIndex == nil || utils.IsNil(inst) {
		return
	}
	member := ""
	// field
	if strings.HasPrefix(name, "#") { // member-call variable contain #, see common/yak/ssa/member_call.go:checkCanMemberCall
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
	if member != "" {
		c.MemberIndex.Add(member, inst.GetId())
		c.enqueueMemberIndex(member, inst)
	} else {
		c.VariableIndex.Add(name, inst.GetId())
		c.enqueueVariableIndex(name, inst)
	}
}

func (c *ProgramCache) RemoveVariable(name string, inst Instruction) {
	if c == nil || utils.IsNil(inst) {
		return
	}
	member := ""
	// field
	if strings.HasPrefix(name, "#") { // member-call variable contain #, see common/yak/ssa/member_call.go:checkCanMemberCall
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
	if member != "" {
		c.MemberIndex.Delete(member, inst.GetId())
	} else {
		c.VariableIndex.Delete(name, inst.GetId())
	}
}

func (c *ProgramCache) AddClassInstance(name string, inst Instruction) {
	if c == nil || c.ClassIndex == nil || utils.IsNil(inst) {
		return
	}
	c.ClassIndex.Add(name, inst.GetId())
	c.enqueueClassIndex(name, inst)
}

// =============================================== Database =======================================================
// only LazyInstruction and false marshal will not be saved to database

func (c *ProgramCache) SaveToDatabase(cb ...func(int)) {
	if !c.HaveDatabaseBackend() {
		return
	}
	if len(cb) > 0 {
		c.afterSaveNotify = cb[0]
	}
	f1 := func() error {
		c.InstructionCache.Close()
		log.Infof("Instruction cache closed")
		return nil
	}
	f2 := func() error {
		c.TypeCache.Close()
		log.Infof("Type Cache closed")
		return nil
	}
	f3 := func() error {
		return nil
	}
	f4 := func() error {
		c.VariableIndex.Close()
		return nil
	}
	f5 := func() error {
		c.MemberIndex.Close()
		return nil
	}
	f6 := func() error {
		c.ClassIndex.Close()
		return nil
	}
	f7 := func() error {
		c.ConstCache.Close()
		return nil
	}
	f8 := func() error {
		c.offsetCache.Close()
		c.editorCache.Close()
		c.indexCache.Close()
		return nil
	}
	f10 := func() error {
		if c.program != nil && c.InstructionCache != nil {
			stats := c.InstructionCache.Stats()
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
		if c.instructionCacheMetrics != nil && c.program != nil {
			c.instructionCacheMetrics.Dump(c.program.GetProgramName())
		}
		return nil
	}
	steps := []func() error{f1, f2, f3, f4, f5, f6, f7, f8, f10}
	c.diagnosticsTrack("ssa.ProgramCache.SaveToDatabase", steps...)
}

func (c *ProgramCache) CountInstruction() int {
	return c.InstructionCache.Count()
}

func (c *ProgramCache) CoolDownFunctionInstructions(function *Function) {
	if c == nil || function == nil || !c.HaveDatabaseBackend() || c.ProgramCacheKind != ProgramCacheDBWrite {
		return
	}

	ids := make([]int64, 0, len(function.Blocks)*8)
	seen := make(map[int64]struct{})
	addID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, blockID := range function.Blocks {
		blockValue, ok := function.GetInstructionById(blockID)
		if !ok || blockValue == nil {
			continue
		}
		block, ok := ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		addID(block.GetId())
		for _, instID := range block.Insts {
			addID(instID)
		}
		for _, phiID := range block.Phis {
			addID(phiID)
		}
	}

	if len(ids) == 0 {
		return
	}
	c.InstructionCache.Track(ids)
}

func (c *ProgramCache) IsExistedSourceCodeHash(programName string, hashString string) bool {
	if programName == "" || !c.HaveDatabaseBackend() {
		return false
	}
	if c.editorHashCache != nil && c.editorHashCache.Have(hashString) {
		return true
	}

	var count int
	if ret := c.DB.Model(&ssadb.IrSource{}).Where(
		"source_code_hash = ?", hashString,
	).Where(
		"program_name = ?", programName,
	).Count(&count).Error; ret != nil {
		log.Warnf("IsExistedSourceCodeHash error: %v", ret)
	}
	return count > 0
}
