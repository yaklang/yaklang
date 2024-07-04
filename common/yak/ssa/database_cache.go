package ssa

import (
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"

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

type instructionIrCode struct {
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
	id               *atomic.Int64
	InstructionCache *utils.CacheWithKey[int64, instructionIrCode] // instructionID to instruction
	VariableCache    *utils.CacheWithKey[string, []Instruction]    // variable(name:string) to []instruction
	Class2InstIndex  map[string][]Instruction
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(programName string, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	ttl := time.Duration(0)
	if databaseEnable {
		// enable database
		ttl = time.Second * 8
	}

	instructionCache := utils.NewTTLCacheWithKey[int64, instructionIrCode](ttl)
	variableCache := utils.NewTTLCacheWithKey[string, []Instruction](ttl)
	cache := &Cache{
		ProgramName:      programName,
		InstructionCache: instructionCache,
		VariableCache:    variableCache,
		Class2InstIndex:  make(map[string][]Instruction),
	}

	if databaseEnable {
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
		instructionCache.SetCheckExpirationCallback(func(key int64, inst instructionIrCode) bool {
			return cache.saveInstruction(inst)
		})
		variableCache.SetCheckExpirationCallback(func(key string, value []Instruction) bool {
			return cache.saveVariable(key, value)
		})
	} else {
		cache.id = atomic.NewInt64(0)
	}

	return cache
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *Cache) SetInstruction(inst Instruction) {
	if inst.GetId() != -1 {
		return
	}

	var id int64
	var instIr instructionIrCode
	if c.DB != nil {
		// use database
		rawID, irCode := ssadb.RequireIrCode(c.DB, c.ProgramName)
		id = int64(rawID)
		instIr = instructionIrCode{
			inst:   inst,
			irCode: irCode,
		}
	} else {
		// not use database
		id = c.id.Inc()
		instIr = instructionIrCode{
			inst:   inst,
			irCode: nil,
		}
	}
	inst.SetId(id)
	c.InstructionCache.Set(id, instIr)
}

func (c *Cache) DeleteInstruction(inst Instruction) {
	c.InstructionCache.Remove(inst.GetId())
}

// GetInstruction : get instruction from cache.
func (c *Cache) GetInstruction(id int64) Instruction {
	ret, ok := c.InstructionCache.Get(id)
	if !ok && c.DB != nil {
		// if no in cache, get from database
		// if found in database, create a new lazy instruction
		return c.newLazyInstruction(id)
		// all instruction from database will be lazy instruction
	}
	return ret.inst
}

// =============================================== Variable =======================================================

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil && c.ProgramName != ""
}

func (c *Cache) GetByVariable(name string) []Instruction {
	ret, ok := c.VariableCache.Get(name)
	if !ok && c.DB != nil {
		// get from database
		irVariable, err := ssadb.GetVariable(c.DB, c.ProgramName, name)
		if err != nil {
			return ret
		}
		ret = lo.Map(irVariable.InstructionID, func(id int64, _ int) Instruction {
			return c.newLazyInstruction(id)
		})
		c.VariableCache.Set(name, ret)
	}
	return ret
}

func (c *Cache) SetVariable(name string, instructions []Instruction) {
	c.VariableCache.Set(name, instructions)
}

func (c *Cache) AddVariable(name string, inst Instruction) {
	c.VariableCache.Set(name, append(c.GetByVariable(name), inst))
}

func (c *Cache) RemoveVariable(name string, inst Instruction) {
	insts := c.GetByVariable(name)
	insts = utils.RemoveSliceItem(insts, inst)
	c.SetVariable(name, insts)
}

func (c *Cache) ForEachVariable(handle func(string, []Instruction)) {
	c.VariableCache.ForEach(handle)
}

func (c *Cache) AddClassInstance(name string, inst Instruction) {
	// log.Errorf("AddClassInstance: %s : %v", name, inst)
	// if _, ok := c.Class2InstIndex[name]; !ok {
	// 	c.Class2InstIndex[name] = make([]Instruction, 0, 1)
	// }
	c.Class2InstIndex[name] = append(c.Class2InstIndex[name], inst)
}

func (c *Cache) GetClassInstance(name string) []Instruction {
	return c.Class2InstIndex[name]
}

var (
	_SSASaveIrCodeCost uint64
)

func GetSSASaveIrCodeCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSASaveIrCodeCost))
}

func ShowDatabaseCacheCost() {
	log.Infof("SSA Database SaveIrCode Cost: %v", GetSSASaveIrCodeCost())
	log.Infof("SSA Database SaveVariable Cost: %v", ssadb.GetSSAVariableCost())
	log.Infof("SSA Database SaveSourceCode Cost: %v", ssadb.GetSSASourceCodeCost())
	log.Infof("SSA Database SaveType Cost: %v", ssadb.GetSSASaveTypeCost())
	log.Infof("SSA Database SaveScope Cost: %v", ssautil.GetSSAScopeTimeCost())
	log.Infof("SSA Database CacheToDatabase Cost: %v", GetSSACacheToDatabaseCost())
	log.Infof("SSA DB Cache DEBUG Cost: %v", GetSSACacheIterationCost())
}

// =============================================== Database =======================================================
// only LazyInstruction and false marshal will not be saved to database
func (c *Cache) saveInstruction(instIr instructionIrCode) bool {
	if c.DB == nil {
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

	if err := c.DB.Save(instIr.irCode).Error; err != nil {
		log.Errorf("Save irCode error: %v", err)
	}

	start := time.Now()
	if r := instIr.inst.GetRange(); r != nil {
		err := ssadb.SaveIrSource(r.GetEditor(), instIr.irCode.SourceCodeHash)
		if err != nil {
			log.Warnf("save source error: %v", err)
		}
	}
	syncAtomic.AddUint64(&_SSASaveIrCodeCost, uint64(time.Now().Sub(start).Nanoseconds()))
	return true
}

func (c *Cache) saveVariable(variable string, insts []Instruction) bool {
	if c.DB == nil {
		log.Errorf("BUG: saveVariable called when DB is nil")
		return false
	}
	if err := ssadb.SaveVariable(c.DB, c.ProgramName, variable, lo.Map(insts, func(ins Instruction, _ int) ssadb.SSAValue {
		return ins
	})); err != nil {
		log.Errorf("SaveVariable error: %v", err)
		return false
	}
	return true
}

func (c *Cache) saveClassInstance(name string, insts []Instruction) {
	if c.DB == nil {
		log.Errorf("BUG: saveClassInstance called when DB is nil")
		return
	}
	if err := ssadb.SaveClassInstance(c.DB, c.ProgramName, name,
		lo.Map(insts, func(inst Instruction, _ int) int64 {
			if inst == nil {
				log.Errorf("BUG: saveClassInstance called with nil instruction %v", name)
				return -1
			}
			return inst.GetId()
		}),
	); err != nil {
		log.Errorf("SaveClassInstance error: %v", err)
	}
}

var (
	_SSACacheToDatabaseCost uint64
	_SSACacheIterationCost  uint64
)

func GetSSACacheToDatabaseCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSACacheToDatabaseCost))
}

func GetSSACacheIterationCost() time.Duration {
	return time.Duration(syncAtomic.LoadUint64(&_SSACacheIterationCost))
}

func (c *Cache) SaveToDatabase() {
	if c.DB == nil {
		return
	}

	start := time.Now()
	defer func() {
		syncAtomic.AddUint64(&_SSACacheToDatabaseCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	insturctionCache := c.InstructionCache.GetAll()
	c.InstructionCache.Close()

	for _, instIR := range insturctionCache {
		c.saveInstruction(instIR)
	}

	variableCache := c.VariableCache.GetAll()
	c.VariableCache.Close()

	insIterStart := time.Now()
	for variable, insts := range variableCache {
		c.saveVariable(variable, insts)
	}
	syncAtomic.AddUint64(&_SSACacheIterationCost, uint64(time.Now().Sub(insIterStart).Nanoseconds()))

	for name, insts := range c.Class2InstIndex {
		c.saveClassInstance(name, insts)
	}
}

func (c *Cache) IsExistedSourceCodeHash(programName string, hashString string) bool {
	if programName == "" {
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
