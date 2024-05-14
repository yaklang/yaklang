package ssa

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"regexp"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

var CachePool = omap.NewEmptyOrderedMap[string, *Cache]()

func GetCacheFromPool(programName string) *Cache {
	if cache, ok := CachePool.Get(programName); ok {
		return cache
	}
	cache := NewDBCache(programName)
	CachePool.Set(programName, cache)
	return cache
}

var DB = consts.GetGormProjectDatabase() // not good here...

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

func (c *Cache) Yield(db *gorm.DB) chan Instruction {
	var ch = make(chan Instruction)
	go func() {
		defer close(ch)

		var pack []*ssadb.IrVariable
		var pg *bizhelper.Paginator
		var page = 1
		for {
			pg, db = bizhelper.Paging(db, page, 100, &pack)
			if pg.TotalPage == 0 {
				break
			}
			for _, inst := range pack {
				for _, id := range inst.InstructionID {
					ch <- c.newLazyInstruction(id)
				}
			}
			if pg.TotalPage == page {
				break
			}
			page++
		}
		db.Find(&pack)
	}()
	return ch
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
		cache.DB = DB
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
func (c *Cache) exactSearchVariable(value string) chan Instruction {
	db := c.DB.Model(&ssadb.IrVariable{}).Where("program_name = ?", c.ProgramName)
	db = db.Where("("+
		"variable_name = ?"+
		"OR slice_member_name = ?"+
		"OR field_member_name = ?"+
		")", value, value, value)
	return c.Yield(db)
}

func (c *Cache) globSearchVariable(value string) chan Instruction {
	db := c.DB.Model(&ssadb.IrVariable{}).Where("program_name = ?", c.ProgramName)
	db = db.Where("("+
		"variable_name GLOB ? "+
		"OR slice_member_name GLOB ? "+
		"OR field_member_name GLOB ?"+
		")", value, value, value)
	return c.Yield(db)
}

func (c *Cache) regexpSearchVariable(value string) chan Instruction {
	db := c.DB.Model(&ssadb.IrVariable{}).Where("program_name = ?", c.ProgramName)
	db = db.Where("("+
		"variable_name REGEXP ?"+
		"OR slice_member_name REGEXP ?"+
		"OR field_member_name REGEXP ?"+
		")", value, value, value)
	return c.Yield(db)
}

func (c *Cache) HaveDatabaseBackend() bool {
	return c.DB != nil && c.ProgramName != ""
}

func (c *Cache) GetByVariable(name string) []Instruction {
	if c.HaveDatabaseBackend() {
		var ins []Instruction
		for i := range c.exactSearchVariable(name) {
			ins = append(ins, i)
		}
		return ins
	}
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

// GetByVariableGlob means get variable name(glob).
func (c *Cache) GetByVariableGlob(g sfvm.Glob) []Instruction {
	if c.HaveDatabaseBackend() {
		var ins []Instruction
		for i := range c.globSearchVariable(g.String()) {
			ins = append(ins, i)
		}
		return ins
	}
	var ins []Instruction
	c.VariableCache.ForEach(func(s string, instructions []Instruction) {
		log.Infof("GetByVariableGlob: %s", s)
		if g.Match(s) {
			ins = append(ins, instructions...)
		}
	})
	return ins
}

// GetByVariableRegexp will filter Instruction via variable regexp name
func (c *Cache) GetByVariableRegexp(r *regexp.Regexp) []Instruction {
	if c.HaveDatabaseBackend() {
		var ins []Instruction
		for i := range c.regexpSearchVariable(r.String()) {
			ins = append(ins, i)
		}
		return ins
	}

	var ins []Instruction
	c.VariableCache.ForEach(func(s string, instructions []Instruction) {
		if r.MatchString(s) {
			ins = append(ins, instructions...)
		}
	})
	return ins
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

	if err := Instruction2IrCode(instIr.inst, instIr.irCode); err != nil {
		log.Errorf("FitIRCode error: %s", err)
		return false
	}
	if err := c.DB.Save(instIr.irCode).Error; err != nil {
		log.Errorf("Save irCode error: %v", err)
	}
	if r := instIr.inst.GetRange(); r != nil {
		err := ssadb.SaveIrSource(c.DB, r.GetEditor(), instIr.irCode.SourceCodeHash)
		if err != nil {
			log.Warnf("save source error: %v", err)
		}
	}
	return true
}

func (c *Cache) saveVariable(variable string, insts []Instruction) bool {
	if c.DB == nil {
		log.Errorf("BUG: saveVariable called when DB is nil")
		return false
	}
	if err := ssadb.SaveVariable(c.DB, c.ProgramName, variable,
		lo.Map(insts, func(inst Instruction, _ int) int64 { return inst.GetId() }),
	); err != nil {
		log.Errorf("SaveVariable error: %v", err)
		return false
	}
	return true
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
