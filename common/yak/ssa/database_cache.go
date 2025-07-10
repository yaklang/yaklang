package ssa

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Cache : a cache in middle layer of database and application.
//
//	application will Get/Set Instruction,
//
// and save the data to database when the data is expired,
// and load the data from database when the data is not in cache.

type ProgramCache struct {
	program          *Program // mark which program handled
	DB               *gorm.DB
	InstructionCache Cache[Instruction]
	TypeCache        Cache[Type]

	VariableIndex InstructionsIndex
	MemberIndex   InstructionsIndex
	ClassIndex    InstructionsIndex
	ConstCache    InstructionsIndex
	OffsetCache   InstructionsIndex

	afterSaveNotify func(int)

	waitGroup *sync.WaitGroup // wait for all goroutines to finish

	// For pre-fetching IDs
	cacheCtxCancel context.CancelFunc
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseKind ProgramCacheKind, fileSize int, ConfigTTL ...time.Duration) *ProgramCache {
	compileCtx := context.Background()
	cacheCtx, cancel := context.WithCancel(compileCtx)
	cache := &ProgramCache{
		program: prog,
		// set ttl
		cacheCtxCancel: cancel,
		waitGroup:      &sync.WaitGroup{},
	}
	var programName string
	if databaseKind != ProgramCacheMemory { // database write/read
		programName = prog.GetApplication().GetProgramName()
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
	}
	fetchSize := min(max(fileSize*5, defaultFetchSize), maxFetchSize)
	saveSize := min(max(fileSize*5, defaultSaveSize), maxSaveSize)
	log.Infof("Databasex Channel: ReSetSize: fileSize(%d) fetchSize(%d) saveSize(%d)", fileSize, fetchSize, saveSize)
	cache.initIndex(databaseKind, saveSize/2)
	cache.afterSaveNotify = func(i int) {}
	cache.InstructionCache = createInstructionCache(
		cacheCtx, databaseKind,
		cache.DB, prog,
		programName, fetchSize, saveSize,
		func(inst Instruction, instIr *ssadb.IrCode) {
			// TODO: this offset too long time
			// cache.OffsetCache.Add("", inst) // add to offset cache
		},
		func(count int) {
			go func() {
				step := 100
				if cache.afterSaveNotify != nil {
					i := 0
					for i <= count {
						cache.afterSaveNotify(step)
						i += step // notify every step instructions
					}
					if rest := count % step; rest != 0 {
						// notify the remaining count if not evenly divisible by step
						cache.afterSaveNotify(rest)
					}
				}
			}()
		},
	)
	cache.TypeCache = createTypeCache(
		cacheCtx, databaseKind,
		cache.DB, prog,
		programName, fetchSize, saveSize,
	)
	return cache
}

func (c *ProgramCache) HaveDatabaseBackend() bool {
	return c.DB != nil
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *ProgramCache) SetInstruction(inst Instruction) {
	if utils.IsNil(inst) {
		log.Errorf("BUG: SetInstruction called with nil instruction")
		return
	}
	f1 := func() {
		c.InstructionCache.Set(inst)
	}
	ProfileAdd(true, "ssa.ProgramCache.SetInstruction", f1)
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

// =============================================== Variable =======================================================

func (c *ProgramCache) AddConst(inst Instruction) {
	f1 := func() {
		c.ConstCache.Add(inst.GetName(), inst)
	}
	ProfileAdd(true, "ssa.ProgramCache.AddConst", f1)
}

func (c *ProgramCache) AddVariable(name string, inst Instruction) {
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
	if member != "" {
		f1 := func() {
			c.MemberIndex.Add(member, inst)
		}
		ProfileAdd(true, "ssa.ProgramCache.AddVariable.Member", f1)
	} else {
		f1 := func() {
			c.VariableIndex.Add(name, inst)
		}
		ProfileAdd(true, "ssa.ProgramCache.AddVariable.Name", f1)
	}
}

func (c *ProgramCache) RemoveVariable(name string, inst Instruction) {
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

	if member != "" {
		c.MemberIndex.Delete(member, inst)
	} else {
		c.VariableIndex.Delete(name, inst)
	}
}

func (c *ProgramCache) AddClassInstance(name string, inst Instruction) {
	f1 := func() {
		c.ClassIndex.Add(name, inst)
	}
	ProfileAdd(true, "ssa.ProgramCache.AddClassInstance", f1)
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
	f1 := func() {
		c.InstructionCache.Close()
	}
	f2 := func() {
		c.TypeCache.Close()
	}
	f3 := func() {
		c.VariableIndex.Close()
	}
	f4 := func() {
		c.MemberIndex.Close()
	}
	f5 := func() {
		c.ClassIndex.Close()
	}
	f6 := func() {
		c.ConstCache.Close()
	}
	f7 := func() {
		c.OffsetCache.Close()
	}
	f8 := func() {
		c.cacheCtxCancel()
	}
	ProfileAdd(true, "ssa.ProgramCache.SaveToDatabase",
		f1, f2, f3, f4, f5, f6, f7, f8)
}

func (c *ProgramCache) CountInstruction() int {
	return c.InstructionCache.Count()
}

func (c *ProgramCache) IsExistedSourceCodeHash(programName string, hashString string) bool {
	if programName == "" || !c.HaveDatabaseBackend() {
		return false
	}

	var count int
	if ret := c.DB.Model(&ssadb.IrCode{}).Where(
		"source_code_hash = ?", hashString,
	).Where(
		"program_name = ?", programName,
	).Count(&count).Error; ret != nil {
		log.Warnf("IsExistedSourceCodeHash error: %v", ret)
	}
	return count > 0
}
