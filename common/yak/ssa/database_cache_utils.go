package ssa

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

type instructionCachePair struct {
	inst   Instruction
	irCode *ssadb.IrCode
}

func (c *instructionCachePair) GetId() int64 {
	return c.irCode.GetIdInt64()
}

const (
	fetchSize = 300
	saveSize  = 2000
	saveTime  = time.Second * 1
	cacheTTL  = 8 * time.Second
	typeTTL   = 500 * time.Millisecond
)

type Cache[T any] interface {
	// get + set
	Get(int64) (T, bool)
	Set(T)
	Delete(int64)

	Count() int
	GetAll() map[int64]T

	// close
	Close()
}

var _ Cache[Instruction] = (*databasex.Cache[Instruction, *ssadb.IrCode])(nil)
var _ Cache[Instruction] = (*memoryCache[Instruction])(nil)

var _ Cache[Type] = (*databasex.Cache[Type, *ssadb.IrType])(nil)
var _ Cache[Type] = (*memoryCache[Type])(nil)

type memoryCache[T databasex.MemoryItem] struct {
	*utils.SafeMapWithKey[int64, T]
	id atomic.Int64
}

func newmemoryCache[T databasex.MemoryItem]() *memoryCache[T] {
	return &memoryCache[T]{
		SafeMapWithKey: utils.NewSafeMapWithKey[int64, T](),
		id:             atomic.Int64{},
	}
}

func (c *memoryCache[T]) Set(item T) {
	id := c.id.Inc()
	c.SafeMapWithKey.Set(id, item)
	item.SetId(id)
}
func (c *memoryCache[T]) Close() {
}

func createInstructionCache(
	databaseEnable bool,
	db *gorm.DB, prog *Program,
	programName string,
	marshalFinish func(Instruction, *ssadb.IrCode),
) Cache[Instruction] {
	if !databaseEnable {
		return newmemoryCache[Instruction]()
	}

	// init instruction cache and fetchId
	fetch := func() []*ssadb.IrCode {
		result := make([]*ssadb.IrCode, 0, fetchSize)
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			// tx := db
			for len(result) < fetchSize {
				id, irCode := ssadb.RequireIrCode(tx, programName)
				if utils.IsNil(irCode) || id <= 0 {
					// return nil // no more id to fetch
					continue
				}
				result = append(result, irCode)
			}
			return nil
		})
		return result
	}

	delete := func(fir []*ssadb.IrCode) {
		ids := lo.Map(fir, func(item *ssadb.IrCode, _ int) int64 {
			return item.GetIdInt64()
		})
		ssadb.DeleteIrCode(db, ids...)
	}

	save := func(t []*ssadb.IrCode) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DATABASE: Save IR Codes panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			log.Errorf("DATABASE: Save IR: %d", len(t))
			for _, irCode := range t {
				if err := irCode.Save(tx); err != nil {
					log.Errorf("DATABASE: save irCode to database error: %v", err)
				}
			}
			return nil
		})

	}

	marshal := func(s Instruction, d *ssadb.IrCode) {
		if marshalInstruction(prog.Cache, s, d) {
			marshalFinish(s, d)
		}
	}
	load := func(id int64) (Instruction, *ssadb.IrCode, error) {
		irCode := ssadb.GetIrCodeById(db, id)
		inst, err := NewLazyInstructionFromIrCode(irCode, prog, true)
		if err != nil {
			return nil, nil, utils.Wrap(err, "NewLazyInstruction failed")
		}
		return inst, irCode, nil
	}

	opts := []databasex.Option{
		databasex.WithBufferSize(fetchSize),
		databasex.WithSaveSize(saveSize),
		databasex.WithSaveTimeout(saveTime),
	}
	return databasex.NewCache(
		cacheTTL, marshal, fetch, delete, save, load, opts...,
	)
}

func createTypeCache(
	databaseEnable bool,
	db *gorm.DB, prog *Program,
	programName string,
) Cache[Type] {
	if !databaseEnable {
		return newmemoryCache[Type]()
	}

	marshal := func(s Type, d *ssadb.IrType) {
		marshalType(s, d)
	}

	fetch := func() []*ssadb.IrType {
		result := make([]*ssadb.IrType, 0, fetchSize)
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			// tx := db
			for len(result) < fetchSize {
				id, irType := ssadb.RequireIrType(tx, programName)
				if utils.IsNil(irType) || id <= 0 {
					// return nil // no more id to fetch
					continue
				}
				result = append(result, irType)
			}
			return nil
		})
		return result
	}

	delete := func(fir []*ssadb.IrType) {
		ids := lo.Map(fir, func(item *ssadb.IrType, _ int) int64 {
			return item.GetIdInt64()
		})
		ssadb.DeleteIrType(db, ids)
	}

	save := func(t []*ssadb.IrType) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DATABASE: Save IR Types panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			log.Errorf("DATABASE: Save IR Types: %d", len(t))
			for _, irType := range t {
				_ = irType
				if err := irType.Save(tx); err != nil {
					log.Errorf("DATABASE: save irType to database error: %v", err)
				}
			}
			return nil
		})
	}

	load := func(id int64) (Type, *ssadb.IrType, error) {
		irType := ssadb.GetIrTypeById(db, id)
		typ := GetTypeFromDB(prog.Cache, id)
		return typ, irType, nil
	}

	opts := []databasex.Option{
		databasex.WithBufferSize(fetchSize),
		databasex.WithSaveSize(saveSize),
		databasex.WithSaveTimeout(saveTime),
		databasex.WithEnableSave(true), // always enable save for type cache
	}

	return databasex.NewCache[Type, *ssadb.IrType](
		typeTTL, marshal, fetch, delete, save, load, opts...,
	)
}
