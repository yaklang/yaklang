package ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
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

	VariableCache   map[string][]Instruction // variable(name:string) to []instruction
	MemberCache     map[string][]Instruction
	Class2InstIndex map[string][]Instruction
	constCache      []Instruction
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(prog *Program, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	ttl := time.Duration(0)
	if databaseEnable {
		if len(ConfigTTL) > 0 {
			ttl = ConfigTTL[0]
		} else {
			ttl = time.Second * 8
		}
	}

	cache := &Cache{
		program:         prog,
		VariableCache:   make(map[string][]Instruction),
		MemberCache:     make(map[string][]Instruction),
		Class2InstIndex: make(map[string][]Instruction),
		constCache:      make([]Instruction, 0),
	}

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

	return cache
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *Cache) SetInstruction(inst Instruction) {
	if inst.GetId() == -1 {
		// new instruction, use new ID
		id, irCode := c.fetchId()
		inst.SetId(id)
		c.InstructionCache.Set(id, &instructionCachePair{
			inst:   inst,
			irCode: irCode,
		})
	} else {
		id := inst.GetId()
		// this cache will auto load from database
		pair, ok := c.InstructionCache.Get(id)
		_ = pair
		_ = ok
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
	c.constCache = append(c.constCache, inst)
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
		c.MemberCache[member] = append(c.MemberCache[member], inst)
	} else {
		c.VariableCache[name] = append(c.VariableCache[name], inst)
	}
	if c.HaveDatabaseBackend() {
		SaveVariableIndex(inst, name, member)
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
		c.MemberCache[member] = utils.RemoveSliceItem(c.MemberCache[member], inst)
	} else {
		c.VariableCache[name] = utils.RemoveSliceItem(c.VariableCache[name], inst)
	}
}

func (c *Cache) AddClassInstance(name string, inst Instruction) {
	c.Class2InstIndex[name] = append(c.Class2InstIndex[name], inst)
	if c.HaveDatabaseBackend() {
		SaveClassIndex(inst, name)
	}
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

	if fun, ok := ToFunction(instIr.inst); ok {
		if !fun.IsBuilded() {
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
	if err := c.DB.Save(instIr.irCode).Error; err != nil {
		log.Errorf("Save irCode error: %v", err)
	}
	syncAtomic.AddUint64(&_SSASaveIrCodeCost, uint64(time.Since(start)))

	return true
}

func (c *Cache) SaveToDatabase() {
	if !c.HaveDatabaseBackend() {
		return
	}
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
