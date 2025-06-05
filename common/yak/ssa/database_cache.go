package ssa

import (
	"reflect"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	syncAtomic "sync/atomic"
)

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

	afterSaveNotify func()
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	ttl := time.Duration(0)
	cache := &Cache{
		program: prog,
	}
	// set ttl
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
		cache.fetchId = func() (int64, *ssadb.IrCode) {
			return ssadb.RequireIrCode(cache.DB, programName)
		}
		save = func(i int64, s *instructionCachePair, reason utils.EvictionReason) bool {
			if reason == utils.EvictionReasonExpired {
				if s.inst.GetOpcode() == SSAOpcodeFunction || s.inst.GetOpcode() == SSAOpcodeBasicBlock {
					// function is not saved to database, because it is not changed
					return false
				}
			}
			return cache.saveInstruction(s)
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

func (c *Cache) Refresh(insts any) {
	if c.InstructionCache.IsClose() {
		return
	}
	if utils.IsNil(insts) {
		return
	}
	refresh := func(inst Instruction) {
		id := inst.GetId()
		if id <= 0 {
			c.SetInstruction(inst)
			return
		}
		if item, ok := c.InstructionCache.GetPure(id); ok {
			if item.inst != inst {
				if item.inst.IsLazy() {
					item.inst = inst
					c.InstructionCache.Set(id, item)
				}
			}
		} else {
			c.InstructionCache.Set(id, &instructionCachePair{
				inst:   inst,
				irCode: ssadb.GetIrCodeById(ssadb.GetDB(), id),
			})
		}
	}
	t := reflect.TypeOf(insts).Kind()
	if t == reflect.Array || t == reflect.Slice {
		len := reflect.ValueOf(insts).Len()
		for i := 0; i < len; i++ {
			if ins, ok := reflect.ValueOf(insts).Index(i).Interface().(Instruction); ok {
				refresh(ins)
			}
		}
	} else {
		if ins, ok := insts.(Instruction); ok {
			refresh(ins)
		}
	}
}

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
	} else {
		c.Refresh(inst)
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
		// log.Infof("add variable %s : %v", name, inst)
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
		// log.Infof("remove variable %s : %v", name, inst)
		c.VariableIndex.Delete(name, inst)
	}
}

func (c *Cache) AddClassInstance(name string, inst Instruction) {
	c.ClassIndex.Add(name, inst)
}

// =============================================== Database =======================================================
// only LazyInstruction and false marshal will not be saved to database
func (c *Cache) saveInstruction(instIr *instructionCachePair) bool {
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
	if err := instIr.irCode.Save(c.DB); err != nil {
		log.Errorf("Save irCode error: %v", err)
	}
	syncAtomic.AddUint64(&_SSASaveIrCodeCost, uint64(time.Since(start)))
	if c.afterSaveNotify != nil {
		c.afterSaveNotify()
	}

	return true
}

func (c *Cache) SaveToDatabase(cb ...func()) {
	if !c.HaveDatabaseBackend() {
		return
	}
	if len(cb) > 0 {
		c.afterSaveNotify = cb[0]
	}
	c.InstructionCache.EnableSave()
	c.InstructionCache.Close()
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
