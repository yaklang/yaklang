package ssa

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
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
	VariableCache    map[string][]Instruction                      // variable(name:string) to []instruction
	Class2InstIndex  map[string][]Instruction
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(programName string, databaseEnable bool, ConfigTTL ...time.Duration) *Cache {
	ttl := time.Duration(0)
	if databaseEnable {
		// enable database
		ttl = time.Second * 8
	}

	cache := &Cache{
		ProgramName:      programName,
		InstructionCache: utils.NewTTLCacheWithKey[int64, instructionIrCode](ttl),
		VariableCache:    make(map[string][]Instruction),
		Class2InstIndex:  make(map[string][]Instruction),
	}

	if databaseEnable {
		cache.DB = ssadb.GetDB().Where("program_name = ?", programName)
		cache.InstructionCache.SetCheckExpirationCallback(func(key int64, inst instructionIrCode) bool {
			return cache.saveInstruction(inst)
		})
	} else {
		cache.id = atomic.NewInt64(0)
	}

	return cache
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil && c.ProgramName != ""
}

// =============================================== Instruction =======================================================

// SetInstruction : set instruction to cache.
func (c *Cache) SetInstruction(inst Instruction) {
	if inst.GetId() != -1 {
		return
	}

	var id int64
	var instIr instructionIrCode
	if c.HaveDatabaseBackend() {
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
	if !ok && c.HaveDatabaseBackend() {
		// if no in cache, get from database
		// if found in database, create a new lazy instruction
		return c.newLazyInstruction(id)
		// all instruction from database will be lazy instruction
	}
	return ret.inst
}

// =============================================== Variable =======================================================

func (c *Cache) GetByVariable(name string) []Instruction {
	ret, ok := c.VariableCache[name]
	if !ok && c.HaveDatabaseBackend() {
		// get from database
		// TODO: this code should be need?
		ret = make([]Instruction, 0)
		for i := range ssadb.ExactSearchVariable(c.DB, ssadb.NameMatch, name) {
			ret = append(ret, c.newLazyInstruction(i))
		}
		c.VariableCache[name] = ret
	}
	return ret
}

func (c *Cache) AddVariable(name string, inst Instruction) {
	data := c.GetByVariable(name)
	data = append(data, inst)
	c.VariableCache[name] = data
	if c.HaveDatabaseBackend() {
		SaveVariableIndex(inst, name)
	}
}

func (c *Cache) RemoveVariable(name string, inst Instruction) {
	insts := c.GetByVariable(name)
	insts = utils.RemoveSliceItem(insts, inst)
	c.VariableCache[name] = insts
}

func (c *Cache) AddClassInstance(name string, inst Instruction) {
	c.Class2InstIndex[name] = append(c.Class2InstIndex[name], inst)
	if c.HaveDatabaseBackend() {
		SaveClassIndex(inst, name)
	}
}

// =============================================== Database =======================================================
// only LazyInstruction and false marshal will not be saved to database
func (c *Cache) saveInstruction(instIr instructionIrCode) bool {
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
	if !c.HaveDatabaseBackend() {
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
}

func (c *Cache) IsExistedSourceCodeHash(programName string, hashString string) bool {
	if programName == "" {
		return false
	}

	var (
		count int64
		err   error
	)
	db := c.DB.Model(&ssadb.IrCode{}).Where(
		"source_code_hash = ?", hashString,
	).Where(
		"program_name = ?", programName,
	)
	if count, err = bizhelper.QueryCount(db, nil); err != nil {
		log.Warnf("IsExistedSourceCodeHash error: %v", err)
	}
	return count > 0
}
