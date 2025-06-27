package ssa

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type fetchedIdResult struct {
	id     int64
	irCode *ssadb.IrCode
}

const fetchIdSize = 100

type instructionCachePair struct {
	inst   Instruction
	irCode *ssadb.IrCode
}

// Cache : a cache in middle layer of database and application.
//
//	application will Get/Set Instruction,
//
// and save the data to database when the data is expired,
// and load the data from database when the data is not in cache.

type Cache struct {
	program          *Program // mark which program handled
	DB               *gorm.DB
	fetchId          func() (int64, *ssadb.IrCode) // fetch a new id
	InstructionCache *databasex.DataBaseCacheWithKey[int64, *instructionCachePair]
	Saver            *databasex.Saver[*ssadb.IrCode]

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

const (
	chanSize = 200
	saveSize = 2000
	saveTime = time.Second * 1
)

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	cacheCtx := context.Background()
	ttl := time.Duration(0)
	cache := &Cache{
		program: prog,
		// set ttl
		fetchIdCancel: func() {},
		waitGroup:     &sync.WaitGroup{},
	}
	var programName string
	if databaseEnable {
		if len(ConfigTTL) > 0 {
			ttl = ConfigTTL[0]
		} else {
			ttl = time.Second * 8
		}
		programName = prog.GetProgramName()
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
	}

	// set index
	if databaseEnable {
		cache.VariableIndex = NewInstructionsIndexDB(
			func(items []InstructionsIndexItem) {
				utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
					for _, item := range items {
						SaveVariableIndexByName(tx, item.Name, item.Inst)
					}
					return nil
				})
			},
		)
		cache.MemberIndex = NewInstructionsIndexDB(
			func(items []InstructionsIndexItem) {
				utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
					for _, item := range items {
						SaveVariableIndexByMember(tx, item.Name, item.Inst)
					}
					return nil
				})
			},
		)

		cache.ClassIndex = NewInstructionsIndexDB(
			func(items []InstructionsIndexItem) {
				utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
					for _, item := range items {
						SaveClassIndex(tx, item.Name, item.Inst)
					}
					return nil
				})
			},
		)

		cache.OffsetCache = NewInstructionsIndexDB(func(iii []InstructionsIndexItem) {
			utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
				for _, item := range iii {
					SaveValueOffset(tx, item.Inst)
					if value, ok := ToValue(item.Inst); ok {
						for _, variable := range value.GetAllVariables() {
							if variable.GetId() <= 0 {
								continue // skip variable without id
							}
							SaveVariableOffset(tx, variable, variable.GetName(), int64(value.GetId()))
						}
					}
				}
				return nil
			})

		})

		cache.ConstCache = NewInstructionsIndexDB(
			func(ii []InstructionsIndexItem) {
			},
		)

	} else {
		cache.VariableIndex = NewInstructionsIndexMem()
		cache.MemberIndex = NewInstructionsIndexMem()
		cache.ClassIndex = NewInstructionsIndexMem()
		cache.ConstCache = NewInstructionsIndexMem()
		cache.OffsetCache = NewInstructionsIndexMem()
	}

	// init instruction cache and fetchId
	var save func(int64, *instructionCachePair, utils.EvictionReason) bool
	var load func(int64) (*instructionCachePair, error)

	if databaseEnable {

		irFetch := databasex.NewFetch(func() []fetchedIdResult {
			result := make([]fetchedIdResult, 0, fetchIdSize)
			utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
				// tx := cache.DB
				for len(result) < fetchIdSize {
					id, irCode := ssadb.RequireIrCode(tx, programName)
					if utils.IsNil(irCode) || id <= 0 {
						// return nil // no more id to fetch
						continue
					}
					result = append(result, fetchedIdResult{
						id:     id,
						irCode: irCode,
					})
				}
				return nil
			})
			return result
		},
			databasex.WithBufferSize(fetchIdSize),
			databasex.WithContext(cacheCtx),
		)

		cache.fetchIdCancel = func() {
			irFetch.Close(func(fir ...fetchedIdResult) {
				ids := lo.Map(fir, func(item fetchedIdResult, _ int) int64 {
					return item.id
				})
				ssadb.DeleteIRCode(cache.DB, ids...)
			})
		}

		cache.fetchId = func() (int64, *ssadb.IrCode) {
			var result fetchedIdResult
			result, err := irFetch.Fetch()
			if err != nil {
				log.Errorf("fetchId error: %v", err)
				return -1, nil
			} else {
				return result.id, result.irCode
			}
		}

		saver := databasex.NewSaver(func(t []*ssadb.IrCode) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("DATABASE: Save IR Codes panic: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
				for _, irCode := range t {
					if err := irCode.Save(tx); err != nil {
						log.Errorf("DATABASE: save irCode to database error: %v", err)
					}
				}
				return nil
			})
			if cache.afterSaveNotify != nil {
				cache.afterSaveNotify(len(t))
			}
		},
			databasex.WithBufferSize(chanSize),
			databasex.WithSaveSize(saveSize),
			databasex.WithSaveTimeout(saveTime),
		)
		cache.Saver = saver

		save = func(i int64, s *instructionCachePair, reason utils.EvictionReason) bool {
			if reason == utils.EvictionReasonExpired {
				if s.inst.GetOpcode() == SSAOpcodeFunction || s.inst.GetOpcode() == SSAOpcodeBasicBlock {
					// function is not saved to database, because it is not changed
					return false
				}
			}
			if cache.marshalInstruction(s) {
				cache.OffsetCache.Add("", s.inst)
				saver.Save(s.irCode)
				return true
			}
			return false
		}
		load = func(id int64) (*instructionCachePair, error) {
			irCode := ssadb.GetIrCodeById(cache.DB, id)
			inst, err := NewLazyInstructionFromIrCode(irCode, cache.program, true)
			if err != nil {
				return nil, utils.Wrap(err, "NewLazyInstruction failed")
			}
			return &instructionCachePair{
				inst:   inst,
				irCode: irCode,
			}, nil
		}
	} else {
		id := atomic.NewInt64(0)

		cache.fetchId = func() (int64, *ssadb.IrCode) {
			return id.Inc(), nil
		}
		load = func(i int64) (*instructionCachePair, error) {
			return nil, utils.Errorf("load from database is disabled")
		}
		save = func(i int64, icp *instructionCachePair, reason utils.EvictionReason) bool {
			return false // disable save to database
		}
	}
	cache.InstructionCache = databasex.NewDatabaseCacheWithKey(ttl, save, load)
	cache.InstructionCache.DisableSave()

	return cache
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *Cache) SetInstruction(inst Instruction) {
	id := inst.GetId()
	_ = id
	if inst.GetId() <= 0 {
		// new instruction, use new ID
		id, irCode := c.fetchId()
		inst.SetId(id)
		if id <= 0 {
			log.Errorf("BUG: fetchId return invalid id: %d", id)
			return
		}

		c.InstructionCache.Set(id, &instructionCachePair{
			inst:   inst,
			irCode: irCode,
		})
	}
	if inst.GetId() > 0 {
		c.InstructionCache.Get(id)
	}

}

func (c *Cache) DeleteInstruction(inst Instruction) {
	c.InstructionCache.Delete(inst.GetId())
}

// GetInstruction : get instruction from cache.
func (c *Cache) GetInstruction(id int64) Instruction {
	if id == 0 {
		return nil
	}
	if ret, ok := c.InstructionCache.Get(id); ok {
		return ret.inst
	}
	return nil
}

// =============================================== Variable =======================================================

func (c *Cache) AddConst(inst Instruction) {
	c.ConstCache.Add(inst.GetName(), inst)
}

func (c *Cache) AddVariable(name string, inst Instruction) {
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

func (c *Cache) RemoveVariable(name string, inst Instruction) {
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

func (c *Cache) AddClassInstance(name string, inst Instruction) {
	c.ClassIndex.Add(name, inst)
}

// =============================================== Database =======================================================
// only LazyInstruction and false marshal will not be saved to database
func (c *Cache) marshalInstruction(instIr *instructionCachePair) bool {
	if utils.IsNil(instIr) || utils.IsNil(instIr.inst) {
		log.Errorf("BUG: marshalInstruction called with nil instruction")
		return false
	}
	if instIr.inst.GetId() == -1 {
		log.Errorf("[BUG]: instruction id is -1: %s", codec.AnyToString(instIr.inst))
		return false
	}
	// log.Infof("save instruction : %v", instIr.inst.GetId())
	if !c.HaveDatabaseBackend() {
		log.Errorf("BUG: saveInstruction called when DB is nil")
		return false
	}

	// all instruction from database will be lazy instruction
	if lz, ok := ToLazyInstruction(instIr.inst); ok {
		// we just check if this lazy-instruction should be saved again?
		if !lz.ShouldSave() {
			return false
		}
	}

	err := Instruction2IrCode(instIr.inst, instIr.irCode)
	if err != nil {
		log.Errorf("FitIRCode error: %s", err)
		return false
	}

	if instIr.irCode.Opcode == 0 {
		log.Errorf("BUG: saveInstruction called with empty opcode: %v", instIr.inst.GetName())
	}
	return true
}

func (c *Cache) SaveToDatabase(cb ...func(int)) {
	if !c.HaveDatabaseBackend() {
		return
	}
	if len(cb) > 0 {
		c.afterSaveNotify = cb[0]
	}
	c.InstructionCache.Close()
	if !utils.IsNil(c.Saver) {
		c.Saver.Close()
	}
	c.VariableIndex.Close()
	c.MemberIndex.Close()
	c.ClassIndex.Close()
	c.ConstCache.Close()
	c.OffsetCache.Close()
	c.fetchIdCancel()
}

func (c *Cache) CountInstruction() int {
	return c.InstructionCache.Count()
}

func (c *Cache) IsExistedSourceCodeHash(programName string, hashString string) bool {
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
