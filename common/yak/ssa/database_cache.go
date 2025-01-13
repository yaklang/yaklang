package ssa

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	syncAtomic "sync/atomic"
)

var cachePool = omap.NewEmptyOrderedMap[string, *Cache]()

func GetCacheFromPool(programName string) *Cache {
	if cache, ok := cachePool.Get(programName); ok {
		return cache
	}
	cache := NewDBCache(programName, true)
	cachePool.Set(programName, cache)
	return cache
}

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
	ProgramName      string // mark which program handled
	DB               *gorm.DB
	fetchId          func() (int64, *ssadb.IrCode) // fetch a new id
	InstructionCache *utils.DataBaseCacheWithKey[int64, *instructionCachePair]

	VariableCache   map[string][]Instruction // variable(name:string) to []instruction
	MemberCache     map[string][]Instruction
	Class2InstIndex map[string][]Instruction
	constCache      []Instruction
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(programName string, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	ttl := time.Duration(0)
	if databaseEnable {
		if len(ConfigTTL) > 0 {
			ttl = ConfigTTL[0]
		} else {
			ttl = time.Second * 8
		}
	}

	cache := &Cache{
		ProgramName:     programName,
		VariableCache:   make(map[string][]Instruction),
		MemberCache:     make(map[string][]Instruction),
		Class2InstIndex: make(map[string][]Instruction),
		constCache:      make([]Instruction, 0),
	}

	var save func(int64, *instructionCachePair) bool
	var load func(int64) (*instructionCachePair, error)
	if databaseEnable {
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
		cache.fetchId = func() (int64, *ssadb.IrCode) {
			return ssadb.RequireIrCode(cache.DB, programName)
		}
		save = func(i int64, s *instructionCachePair) bool {
			return cache.saveInstruction(s)
		}
		load = func(i int64) (*instructionCachePair, error) {
			irCode := ssadb.GetIrCodeById(cache.DB, i)
			inst, err := NewLazyInstructionPureFromIr(irCode, cache)
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
		save = func(i int64, icp *instructionCachePair) bool {
			return false // disable save to database
		}
	}
	cache.InstructionCache = utils.NewDatabaseCacheWithKey(ttl, save, load)

	return cache
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil && c.ProgramName != ""
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
	log.Errorf("GetInstruction: %d", id)
	if ret, ok := c.InstructionCache.Get(id); ok {
		return ret.inst
	}
	if !c.HaveDatabaseBackend() || id <= 0 {
		return nil
	}

	// if no in cache, get from database
	// if found in database, create a new lazy instruction
	// return c.newLazyInstructionWithoutCache(id)
	v, err := newLazyInstruction(id, nil, c)
	if err != nil {
		log.Errorf("newLazyInstruction failed: %v", err)
		return nil
	}
	return v
	// all instruction from database will be lazy instruction
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

	log.Errorf("save instructon %d", instIr.inst.GetId())
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
