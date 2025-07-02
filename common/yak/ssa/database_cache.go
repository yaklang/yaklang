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
	fetchIdCancel context.CancelFunc
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseEnable bool, ConfigTTL ...time.Duration) *ProgramCache {
	cache := &ProgramCache{
		program: prog,
		// set ttl
		fetchIdCancel: func() {},
		waitGroup:     &sync.WaitGroup{},
	}
	var programName string
	if databaseEnable {
		programName = prog.GetApplication().GetProgramName()
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
	}

	cache.initIndex(databaseEnable)
	cache.afterSaveNotify = func(i int) {}
	cache.InstructionCache = createInstructionCache(
		databaseEnable,
		cache.DB, prog,
		programName,
		func(inst Instruction, instIr *ssadb.IrCode) {
			cache.OffsetCache.Add("", inst) // add to offset cache
			cache.afterSaveNotify(1)        // notify after save
		},
	)
	cache.TypeCache = createTypeCache(
		databaseEnable,
		cache.DB, prog,
		programName,
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

// =============================================== Variable =======================================================

func (c *ProgramCache) AddConst(inst Instruction) {
	c.ConstCache.Add(inst.GetName(), inst)
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
		c.MemberIndex.Add(member, inst)
	} else {
		c.VariableIndex.Add(name, inst)
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
	c.ClassIndex.Add(name, inst)
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
	c.InstructionCache.Close()
	c.TypeCache.Close()
	c.VariableIndex.Close()
	c.MemberIndex.Close()
	c.ClassIndex.Close()
	c.ConstCache.Close()
	c.OffsetCache.Close()
	c.fetchIdCancel()
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
