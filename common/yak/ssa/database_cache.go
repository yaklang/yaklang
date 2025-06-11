package ssa

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	syncAtomic "sync/atomic"
)

type fetchedIdResult struct {
	id     int64
	irCode *ssadb.IrCode
}

const fetchIdSize = 50

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
	InstructionCache *utils.DataBaseCacheWithKey[int64, *instructionCachePair]

	VariableIndex InstructionsIndex
	MemberIndex   InstructionsIndex
	ClassIndex    InstructionsIndex
	ConstCache    InstructionsIndex

	afterSaveNotify func(int)

	IrCodeChan      chan *ssadb.IrCode
	IrCodeWaitGroup sync.WaitGroup

	// For pre-fetching IDs
	fetchIdCancel context.CancelFunc
}

var ChanSize = 50

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	cacheCtx := context.Background()
	ttl := time.Duration(0)
	cache := &Cache{
		program: prog,
		// set ttl
		IrCodeChan:      make(chan *ssadb.IrCode, ChanSize),
		IrCodeWaitGroup: sync.WaitGroup{},
		fetchIdCancel:   func() {},
	}
	if databaseEnable {
		if len(ConfigTTL) > 0 {
			ttl = ConfigTTL[0]
		} else {
			ttl = time.Second * 8
		}
	}

	// init instruction cache and fetchId
	var save func(int64, *instructionCachePair, utils.EvictionReason) bool
	var load func(int64) (*instructionCachePair, error)

	if databaseEnable {
		programName := prog.GetProgramName()
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)

		fetchIdChan := make(chan fetchedIdResult, fetchIdSize)
		// Start the ID pre-fetcher goroutine
		fetchIdCtx, fetchIdCancel := context.WithCancel(cacheCtx)
		cache.fetchIdCancel = fetchIdCancel
		show := func() {
			if len(fetchIdChan) < fetchIdSize/2 {
				log.Errorf("fetchIdChan is too small: %d, program: %s", len(fetchIdChan), programName)
			}
		}
		go func() {
			defer close(fetchIdChan)
			for {
				show()
				select {
				case <-fetchIdCtx.Done():
					result := make([]int, 0, fetchIdSize)
					for len(fetchIdChan) > 0 {
						res := <-fetchIdChan
						result = append(result, int(res.id))
					}
					ssadb.DeleteIRCode(cache.DB, result...)
					return
				default:
					loadSize := 5
					result := make([]fetchedIdResult, 0, loadSize)
					// log.Errorf("load from db with transaction ")
					utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
						for i := 0; i < loadSize; i++ {
							// log.Errorf("load from db with transaction: %v ", i)
							id, irCode := ssadb.RequireIrCode(tx, programName)
							if irCode == nil || id <= 0 {
								continue
							}
							result = append(result, fetchedIdResult{id: id, irCode: irCode})
						}
						return nil
					})

					// log.Errorf("load from db with transaction done ")
					for _, res := range result {
						select {
						case fetchIdChan <- res:
						case <-fetchIdCtx.Done():
							ssadb.DeleteIRCode(cache.DB, int(res.id))
						}
					}
				}
			}
		}()

		cache.fetchId = func() (int64, *ssadb.IrCode) {
			start := time.Now()
			defer func() {
				syncAtomic.AddUint64(&FetchInstructionTime, uint64(time.Since(start)))
				syncAtomic.AddUint64(&FetchInstructionCount, 1)
			}()
			result := <-fetchIdChan
			return result.id, result.irCode
		}

		save = func(i int64, s *instructionCachePair, reason utils.EvictionReason) bool {
			if reason == utils.EvictionReasonExpired {
				if s.inst.GetOpcode() == SSAOpcodeFunction || s.inst.GetOpcode() == SSAOpcodeBasicBlock {
					// function is not saved to database, because it is not changed
					return false
				}
			}
			if cache.marshalInstruction(s) {
				cache.IrCodeChan <- s.irCode
				return true
			}
			return false
		}
		load = func(id int64) (*instructionCachePair, error) {
			start := time.Now()
			defer func() {
				syncAtomic.AddUint64(&LoadInstructionTime, uint64(time.Since(start)))
				syncAtomic.AddUint64(&LoadInstructionCount, 1)
			}()
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

		cache.IrCodeWaitGroup.Add(1)
		go func() {
			defer cache.IrCodeWaitGroup.Done()

			irCodes := make([]*ssadb.IrCode, 0, 100)
			save := func() {
				if len(irCodes) == 0 {
					return
				}
				start := time.Now()
				utils.GormTransaction(cache.DB, func(tx *gorm.DB) error {
					for _, irCode := range irCodes {
						if err := irCode.Save(tx); err != nil {
							log.Errorf("save irCode to database error: %v", err)
						}
					}
					return nil
				})
				syncAtomic.AddUint64(&_SSASaveIrCodeDBCost, uint64(time.Since(start)))
				syncAtomic.AddUint64(&_SSASaveIrCodeDBCount, 1)
				if cache.afterSaveNotify != nil {
					cache.afterSaveNotify(len(irCodes))
				}
				irCodes = make([]*ssadb.IrCode, 0, 100)
			}
			for irCode := range cache.IrCodeChan {

				if len(irCodes) == 100 {
					save()
				} else {
					irCodes = append(irCodes, irCode)
				}
			}
			save()
		}()

	} else {
		id := atomic.NewInt64(0)

		cache.fetchId = func() (int64, *ssadb.IrCode) {
			start := time.Now()
			defer func() {
				syncAtomic.AddUint64(&FetchInstructionTime, uint64(time.Since(start)))
				syncAtomic.AddUint64(&FetchInstructionCount, 1)
			}()
			return id.Inc(), nil
		}
		load = func(i int64) (*instructionCachePair, error) {
			return nil, utils.Errorf("load from database is disabled")
		}
		save = func(i int64, icp *instructionCachePair, reason utils.EvictionReason) bool {
			return false // disable save to database
		}
	}
	cache.InstructionCache = utils.NewDatabaseCacheWithKey(ttl, save, load)
	cache.InstructionCache.DisableSave()

	// set index
	if databaseEnable {
		cache.VariableIndex = NewInstructionsIndexDB(
			func(s string, i Instruction) error {
				SaveVariableIndexByName(s, i)
				return nil
			},
		)
		cache.MemberIndex = NewInstructionsIndexDB(
			func(s string, i Instruction) error {
				SaveVariableIndexByMember(s, i)
				return nil
			},
		)

		cache.ClassIndex = NewInstructionsIndexDB(
			func(s string, i Instruction) error {
				SaveClassIndex(s, i)
				return nil
			},
		)

		cache.ConstCache = NewInstructionsIndexDB(
			func(s string, i Instruction) error {
				return nil
			},
		)

	} else {
		cache.VariableIndex = NewInstructionsIndexMem()
		cache.MemberIndex = NewInstructionsIndexMem()
		cache.ClassIndex = NewInstructionsIndexMem()
		cache.ConstCache = NewInstructionsIndexMem()
	}

	return cache
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *Cache) SetInstruction(inst Instruction) {
	{
		start := time.Now()
		defer func() {
			syncAtomic.AddUint64(&SetInstructionTime, uint64(time.Since(start)))
			syncAtomic.AddUint64(&SetInstructionCount, 1)
		}()
	}

	var start time.Time
	id := inst.GetId()
	_ = id

	if inst.GetId() <= 0 {
		// new instruction, use new ID
		start = time.Now()
		id, irCode := c.fetchId()
		syncAtomic.AddUint64(&Site1, uint64(time.Since(start)))
		inst.SetId(id)
		if id <= 0 {
			log.Errorf("BUG: fetchId return invalid id: %d", id)
			return
		}

		start = time.Now()
		c.InstructionCache.Set(id, &instructionCachePair{
			inst:   inst,
			irCode: irCode,
		})
		syncAtomic.AddUint64(&Site2, uint64(time.Since(start)))
	} else {
		start = time.Now()
		c.InstructionCache.Get(id)
		syncAtomic.AddUint64(&Site3, uint64(time.Since(start)))
	}
}

func (c *Cache) DeleteInstruction(inst Instruction) {
	c.InstructionCache.Delete(inst.GetId())
}

// GetInstruction : get instruction from cache.
func (c *Cache) GetInstruction(id int64) Instruction {
	start := time.Now()
	defer func() {
		syncAtomic.AddUint64(&GetInstructionTime, uint64(time.Since(start)))
		syncAtomic.AddUint64(&GetInstructionCount, 1)
	}()
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
		log.Infof("add member %s : %v", name, inst)
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
		log.Infof("remove member %s : %v", name, inst)
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
	if instIr.inst.GetId() == -1 {
		log.Errorf("[BUG]: instruction id is -1: %s", codec.AnyToString(instIr.inst))
		return false
	}
	// log.Infof("save instruction : %v", instIr.inst.GetId())
	start := time.Now()
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
	syncAtomic.AddUint64(&_SSASaveIrCodeCPUCost, uint64(time.Since(start)))
	syncAtomic.AddUint64(&_SSASaveIrCodeCPUCount, 1)
	return true
}

func (c *Cache) SaveToDatabase(cb ...func(int)) {
	start := time.Now()
	syncAtomic.AddUint64(&SaveDBWait, uint64(time.Since(start)))
	c.fetchIdCancel()
	// if !c.HaveDatabaseBackend() {
	// 	return
	// }
	if len(cb) > 0 {
		c.afterSaveNotify = cb[0]
	}
	c.InstructionCache.Close()
	close(c.IrCodeChan)
	c.IrCodeWaitGroup.Wait()
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
