package ssa

import (
	"context"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

type ProgramCacheKind int

const (
	ProgramCacheMemory  ProgramCacheKind = iota
	ProgramCacheDBRead                   // fetch and save mode  // for compile
	ProgramCacheDBWrite                  // only load from database // for scan
)

type instructionCachePair struct {
	inst   Instruction
	irCode *ssadb.IrCode
}

func (c *instructionCachePair) GetId() int64 {
	return c.irCode.GetIdInt64()
}

const (
	defaultFetchSize = 2
	maxFetchSize     = 40000
	defaultSaveSize  = 200
	maxSaveSize      = 40000
	saveTime         = time.Second * 1
	cacheTTL         = 8 * time.Second
	typeTTL          = 500 * time.Millisecond
	batch            = 999 // batch size for sqlite

)

type Cache[T any] interface {
	// get + set
	Get(int64) (T, bool)
	Set(T)
	Delete(int64)

	Count() int
	GetAll() map[int64]T

	// close
	Close(...*sync.WaitGroup)
}

var _ Cache[Instruction] = (*databasex.Cache[Instruction, *ssadb.IrCode])(nil)
var _ Cache[Instruction] = (*memoryCache[Instruction])(nil)

var _ Cache[Type] = (*databasex.Cache[Type, *ssadb.IrType])(nil)
var _ Cache[Type] = (*memoryCache[Type])(nil)

type memoryCache[T databasex.MemoryItem] struct {
	*utils.SafeMapWithKey[int64, T]
	id *atomic.Int64
}

func newmemoryCache[T databasex.MemoryItem]() *memoryCache[T] {
	return &memoryCache[T]{
		SafeMapWithKey: utils.NewSafeMapWithKey[int64, T](),
		id:             atomic.NewInt64(0),
	}
}

func (c *memoryCache[T]) Set(item T) {
	id := c.id.Inc()
	c.SafeMapWithKey.Set(id, item)
	item.SetId(id)
}
func (c *memoryCache[T]) Close(...*sync.WaitGroup) {
}

func createInstructionCache(
	ctx context.Context,
	databaseKind ProgramCacheKind,
	db *gorm.DB, prog *Program,
	programName string, fetchSize, saveSize int,
	marshalFinish func(Instruction, *ssadb.IrCode),
	saveFinish func(int),
) Cache[Instruction] {
	if databaseKind == ProgramCacheMemory {
		return newmemoryCache[Instruction]()
	}
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	fetchSize = min(max(fetchSize, defaultFetchSize), maxFetchSize)

	load := func(id int64) (Instruction, *ssadb.IrCode, error) {
		// TODO: load instruction from db should fix n+1 problem
		irCode := ssadb.GetIrCodeById(db, id)
		inst, err := NewLazyInstructionFromIrCode(irCode, prog, true)
		if err != nil {
			return nil, nil, utils.Wrap(err, "NewLazyInstruction failed")
		}
		return inst, irCode, nil
	}

	var fetch databasex.FetchFunc[*ssadb.IrCode]
	var delete databasex.DeleteFunc[*ssadb.IrCode]
	var save databasex.SaveFunc[*ssadb.IrCode]
	var marshal databasex.MarshalFunc[Instruction, *ssadb.IrCode]

	if databaseKind == ProgramCacheDBWrite {
		// init instruction fetchId and marshal and save
		fetch = func(ctx context.Context, size int) <-chan *ssadb.IrCode {
			if size < defaultFetchSize {
				size = defaultFetchSize // ensure at least fetchSize items are fetched
			}
			// ch := chanx.NewUnlimitedChan[*ssadb.IrCode](ctx, size)
			ch := make(chan *ssadb.IrCode, size)
			go func() {
				defer close(ch)
				utils.GormTransaction(db, func(tx *gorm.DB) error {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("DATABASE: Fetch IR Code panic: %v", err)
						}
					}()
					for i := 0; i < size; i++ {
						select {
						case <-ctx.Done():
							return nil
						default:
							id, irCode := ssadb.RequireIrCode(tx, programName)
							if utils.IsNil(irCode) || id <= 0 {
								// return nil // no more id to fetch
								continue
							}
							ch <- (irCode)
						}
					}
					return nil
				})
			}()
			return ch
		}

		delete = func(fir []*ssadb.IrCode) {
			var ids []int64
			ids = lo.Map(fir, func(item *ssadb.IrCode, _ int) int64 {
				return item.GetIdInt64()
			})
			log.Errorf("DATABASE: irCode delete from db : %d", len(ids))
			ssadb.DeleteIrCode(db, ids...)
		}

		save = func(t []*ssadb.IrCode) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("DATABASE: Save IR Codes panic: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			log.Errorf("Databasex Channel: Save IR  : %d", len(t))
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				// log.Errorf("DATABASE: Save IR: %d", len(t))
				for _, irCode := range t {
					if err := irCode.Save(tx); err != nil {
						log.Errorf("DATABASE: save irCode to database error: %v", err)
					}
					go saveFinish(1)
				}
				return nil
			})
			// log.Errorf("DATABASE: Save IR finish : %d", len(t))
		}

		marshal = func(s Instruction, d *ssadb.IrCode) {
			// log.Errorf("DATABASE: marshal instruction: %v", d.ID)
			success := marshalInstruction(prog.Cache, s, d)
			// log.Errorf("DATABASE: marshal instruction finish : %v, success: %v", d.ID, success)
			if success {
				go marshalFinish(s, d)
			}
		}
	}
	opts := []databasex.Option{
		databasex.WithFetchSize(fetchSize * 2),
		databasex.WithSaveSize(saveSize),
		databasex.WithSaveTimeout(saveTime),
		databasex.WithName("Instruction"),
	}
	return databasex.NewCache(
		cacheTTL, marshal, fetch, delete, save, load, opts...,
	)
}

func createTypeCache(
	ctx context.Context,
	databaseKind ProgramCacheKind,
	db *gorm.DB, prog *Program,
	programName string,
	fetchSize, saveSize int,
) Cache[Type] {
	if databaseKind == ProgramCacheMemory {
		return newmemoryCache[Type]()
	}
	fetchSize = min(max(fetchSize, defaultFetchSize), maxFetchSize)
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)

	load := func(id int64) (Type, *ssadb.IrType, error) {
		irType := ssadb.GetIrTypeById(db, id)
		typ := GetTypeFromDB(prog.Cache, id)
		return typ, irType, nil
	}
	var fetch databasex.FetchFunc[*ssadb.IrType]
	var delete databasex.DeleteFunc[*ssadb.IrType]
	var marshal databasex.MarshalFunc[Type, *ssadb.IrType]
	var save databasex.SaveFunc[*ssadb.IrType]
	if databaseKind == ProgramCacheDBWrite {
		marshal = func(s Type, d *ssadb.IrType) {
			// log.Infof("SAVE: marshal type: %v", d.ID)
			marshalType(s, d)
			// log.Infof("SAVE: marshal type finish : %v", d.ID)
		}

		fetch = func(ctx context.Context, size int) <-chan *ssadb.IrType {
			if size < defaultFetchSize {
				size = defaultFetchSize // ensure at least fetchSize items are fetched
			}
			// ch := chanx.NewUnlimitedChan[*ssadb.IrType](ctx, size)
			ch := make(chan *ssadb.IrType, size)
			go func() {
				defer close(ch)
				utils.GormTransaction(db, func(tx *gorm.DB) error {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("DATABASE: Fetch IR Types panic: %v", err)
						}
					}()
					// db := tx
					for i := 0; i < size; i++ {
						select {
						case <-ctx.Done():
							return nil
						default:
							id, irType := ssadb.RequireIrType(tx, programName)
							if utils.IsNil(irType) || id <= 0 {
								continue
							}
							ch <- (irType)
						}
					}
					return nil
				})
			}()
			return ch
		}

		delete = func(fir []*ssadb.IrType) {
			var ids []int64
			ids = lo.Map(fir, func(item *ssadb.IrType, _ int) int64 {
				return item.GetIdInt64()
			})
			ssadb.DeleteIrType(db, ids)
		}

		save = func(t []*ssadb.IrType) {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("DATABASE: Save IR Types panic: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			// log.Errorf("DATABASE: type save to db : %d", len(t))
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				// log.Errorf("DATABASE: Save IR Types: %d", len(t))
				for _, irType := range t {
					_ = irType
					if err := irType.Save(tx); err != nil {
						log.Errorf("DATABASE: save irType to database error: %v", err)
					}
				}
				// log.Errorf("DATABASE: Save IR Types finish : %d", len(t))
				return nil
			})
		}
	}

	opts := []databasex.Option{
		databasex.WithFetchSize(fetchSize),
		databasex.WithSaveSize(saveSize),
		databasex.WithSaveTimeout(saveTime),
		databasex.WithEnableSave(true), // always enable save for type cache
		databasex.WithName("Type"),
	}

	return databasex.NewCache(
		typeTTL, marshal, fetch, delete, save, load, opts...,
	)
}
