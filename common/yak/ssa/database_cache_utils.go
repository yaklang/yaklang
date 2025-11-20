package ssa

import (
	"context"
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
	ProgramCacheNone ProgramCacheKind = iota
	ProgramCacheMemory
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

func (c *Cache[T]) Close() {
	if c.persistence == nil {
		return
	}

	c.ForEach(func(key int64, value T) bool {
		c.persistence.Save(value)
		return true
	})
	c.persistence.Close()
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
			saveIrCode(prog, db, saveFinish),
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
	prog *Program,
	programName string, saveSize int,

) *Cache[Type] {
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)
	ret := NewCache[Type]()
	ret.SetPersistence(NewSerializingPersistenceStrategy[Type, *ssadb.IrType](ctx,
		marshalIrType(programName), saveIrType(prog, db),
		asyncdb.WithSaveSize(saveSize),
		asyncdb.WithSaveTimeout(saveTime),
		asyncdb.WithEnableSave(true), // always enable save for type cache
		asyncdb.WithName("Type"),
	))
	return ret
}

func saveIrCode(prog *Program, db *gorm.DB, f func(int)) func(t []*ssadb.IrCode) {
	return func(t []*ssadb.IrCode) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DATABASE: Save IR Codes panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		saveStep := func() {
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irCode := range t {
					if err := tx.Save(irCode).Error; err != nil {
						log.Errorf("DATABASE: save irCode to database error: %v", err)
					}
				}
				return nil
			})
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrCodeBatch", saveStep)
		} else {
			saveStep()
		}
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
		if s.GetId() <= 0 {
			log.Errorf("[BUG] marshalIrType: type ID is invalid: %d, type: %s",
				s.GetId(), s.String())
		}

		ret := ssadb.EmptyIrType(name, uint64(s.GetId()))
		marshalType(s, ret)
		return ret, nil
	}
}

func saveIrType(prog *Program, db *gorm.DB) func(t []*ssadb.IrType) {
	return func(t []*ssadb.IrType) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("DATABASE: Save IR Types panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		saveStep := func() {
			utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irType := range t {
					if err := tx.Save(irType).Error; err != nil {
						log.Errorf("DATABASE: save irType to database error: %v", err)
					}
				}
				return nil
			})
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrTypeBatch", saveStep)
		} else {
			saveStep()
		}
	}
}

type PersistenceStrategy[T any] interface {
	Save(item T)
	Close()
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

func (s *SerializingPersistenceStrategy[T, D]) Close() {
	if s == nil {
		return
	}
	s.pipe.Close()
	s.save.Close()
}
