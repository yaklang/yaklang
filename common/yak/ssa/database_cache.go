package ssa

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

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
}

// NewDBCache : create a new ssa db cache. if ttl is 0, the cache will never expire, and never save to database.
func NewDBCache(programName string, ConfigTTL ...time.Duration) *Cache {
	databaseEnable := programName != ""
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
	}

	if databaseEnable {
		cache.DB = consts.GetGormProjectDatabase() // just use the default database
		instructionCache.SetExpirationCallback(func(key int64, instIrCode instructionIrCode) {
			cache.saveInstruction(instIrCode)
		})
		variableCache.SetExpirationCallback(func(key string, insts []Instruction) {
			cache.saveVariable(key, insts)
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
		ir := ssadb.GetIrCodeById(c.DB, id)
		_ = ir
		// ir to Instruction
	}
	return ret.inst
}

// =============================================== Variable =======================================================

// SetVariable : set variable to cache.
func (c *Cache) GetByVariable(name string) []Instruction {
	ret, ok := c.VariableCache.Get(name)
	if !ok && c.DB != nil {
		// get from database
	}
	if ret == nil {
		ret = make([]Instruction, 0)
	}
	return ret
}

// GetByVariable : get variable from cache.
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

// =============================================== Database =======================================================
func (c *Cache) saveInstruction(instIr instructionIrCode) {
	if c.DB == nil {
		return
	}
	if err := FitIRCode(instIr.irCode, instIr.inst); err != nil {
		log.Errorf("FitIRCode error: %s", err)
		return
	}
	if err := c.DB.Save(instIr.irCode).Error; err != nil {
		log.Errorf("Save irCode error: %v", err)
	}
}

func (c *Cache) saveVariable(variable string, insts []Instruction) {
	if c.DB == nil {
		return
	}
	if err := ssadb.SaveVariable(c.DB, c.ProgramName, variable,
		lo.Map(insts, func(inst Instruction, _ int) int64 { return inst.GetId() }),
	); err != nil {
		log.Errorf("SaveVariable error: %v", err)
		return
	}
}
func (c *Cache) SaveToDatabase() {
	if c.DB == nil {
		return
	}
	insturctionCache := c.InstructionCache.GetAll()
	c.InstructionCache.Close()
	for _, instIR := range insturctionCache {
		c.saveInstruction(instIR)
	}

	variableCache := c.VariableCache.GetAll()
	c.VariableCache.Close()
	for variable, insts := range variableCache {
		c.saveVariable(variable, insts)
	}
}
