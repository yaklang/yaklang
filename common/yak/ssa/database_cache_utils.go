package ssa

import (
	"context"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asyncdb"
	"github.com/yaklang/yaklang/common/utils/pipeline"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"go.uber.org/atomic"
)

type ProgramCacheKind int

const (
	ProgramCacheMemory ProgramCacheKind = iota
	// only load from database // for scan
	ProgramCacheDBRead
	// fetch and save mode  // for compile
	ProgramCacheDBWrite
)

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

type Cache[T asyncdb.MemoryItem] struct {
	*utils.SafeMapWithKey[int64, T]
	id *atomic.Int64

	persistence PersistenceStrategy[T]
}

func NewCache[T asyncdb.MemoryItem]() *Cache[T] {
	return &Cache[T]{
		SafeMapWithKey: utils.NewSafeMapWithKey[int64, T](),
		id:             atomic.NewInt64(1),
	}
}

func (c *Cache[T]) SetPersistence(p PersistenceStrategy[T]) {
	c.persistence = p
}

func (c *Cache[T]) Set(item T) {
	if utils.IsNil(item) {
		return
	}
	id := item.GetId()
	if id <= 0 {
		id = c.id.Inc()
		// log.Infof("Cache: assign new id %d to item", id)
	}
	item.SetId(id)
	c.SafeMapWithKey.Set(id, item)
}

func (c *Cache[T]) Close(wg *sync.WaitGroup) {
	if c.persistence == nil {
		return
	}

	c.ForEach(func(key int64, value T) bool {
		c.persistence.Save(value)
		return true
	})
	c.persistence.Close(wg)
}

func createInstructionCache(
	ctx context.Context,
	databaseKind ProgramCacheKind,
	db *gorm.DB, prog *Program,
	programName string, fetchSize, saveSize int,
	saveFinish func(int),
) *Cache[Instruction] {
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	ret := NewCache[Instruction]()
	ret.SetPersistence(
		NewSerializingPersistenceStrategy[Instruction, *ssadb.IrCode](ctx,
			marshalIrCode,
			saveIrCode(db, saveFinish),
			asyncdb.WithSaveSize(saveSize),
			asyncdb.WithSaveTimeout(saveTime),
			asyncdb.WithName("Instruction"),
		),
	)
	return ret
}

func createTypeCache(
	ctx context.Context,
	db *gorm.DB,
	programName string, saveSize int,

) *Cache[Type] {
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	ret := NewCache[Type]()
	ret.SetPersistence(NewSerializingPersistenceStrategy[Type, *ssadb.IrType](ctx,
		marshalIrType(programName), saveIrType(db),
		asyncdb.WithSaveSize(saveSize),
		asyncdb.WithSaveTimeout(saveTime),
		asyncdb.WithEnableSave(true), // always enable save for type cache
		asyncdb.WithName("Type"),
	))
	return ret
}

func saveIrCode(db *gorm.DB, f func(int)) func(t []*ssadb.IrCode) {
	return func(t []*ssadb.IrCode) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DATABASE: Save IR Codes panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			// log.Errorf("DATABASE: Save IR: %d", len(t))
			for _, irCode := range t {
				if err := irCode.Save(tx); err != nil {
					log.Errorf("DATABASE: save irCode to database error: %v", err)
				}
			}
			return nil
		})
		f(len(t))
	}
}

func marshalIrCode(s Instruction) (*ssadb.IrCode, error) {
	ret := ssadb.EmptyIrCode(s.GetProgramName(), s.GetId())
	marshalInstruction(s, ret)
	return ret, nil
}

func marshalIrType(name string) func(s Type) (*ssadb.IrType, error) {
	return func(s Type) (*ssadb.IrType, error) {
		ret := ssadb.EmptyIrType(name, uint64(s.GetId()))
		marshalType(s, ret)
		return ret, nil
	}
}

func saveIrType(db *gorm.DB) func(t []*ssadb.IrType) {
	return func(t []*ssadb.IrType) {
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

type PersistenceStrategy[T any] interface {
	Save(item T)
	Close(wg ...*sync.WaitGroup)
}

var _ PersistenceStrategy[any] = (*asyncdb.Save[any])(nil)

type SerializingPersistenceStrategy[T any, D any] struct {
	pipe *pipeline.Pipe[T, *struct{}]
	save *asyncdb.Save[D]
}

var _ PersistenceStrategy[any] = (*SerializingPersistenceStrategy[any, any])(nil)

func NewSerializingPersistenceStrategy[T any, D any](
	ctx context.Context,
	serialize func(T) (D, error),
	saveFunc func([]D),
	opt ...asyncdb.Option,
) *SerializingPersistenceStrategy[T, D] {
	saver := asyncdb.NewSave(saveFunc, opt...)
	serializePipe := pipeline.NewPipe(ctx, defaultSaveSize, func(item T) (*struct{}, error) {
		if data, err := serialize(item); err == nil {
			saver.Save(data)
			return nil, nil
		} else {
			return nil, err
		}
	})
	return &SerializingPersistenceStrategy[T, D]{
		pipe: serializePipe,
		save: saver,
	}
}

func (s *SerializingPersistenceStrategy[T, D]) Save(item T) {
	if utils.IsNil(item) {
		return
	}
	s.pipe.Feed(item)
}

func (s *SerializingPersistenceStrategy[T, D]) Close(wg ...*sync.WaitGroup) {
	if s == nil {
		return
	}
	s.pipe.Close()
	s.save.Close(wg...)
}
