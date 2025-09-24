package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asyncdb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type SimpleCache[T comparable] interface {
	Delete(string, T)
	Add(string, T)
	ForEach(func(string, []T))
	Close()
}

var _ SimpleCache[any] = (*simpleCacheMemory[any])(nil)
var _ SimpleCache[any] = (*simpleCacheDB[any])(nil)

type simpleCacheMemory[T comparable] struct {
	name  string
	cache *utils.SafeMapWithKey[string, []T]
}

func NewSimpleCacheMemory[T comparable](name string) *simpleCacheMemory[T] {
	return &simpleCacheMemory[T]{
		name:  name,
		cache: utils.NewSafeMapWithKey[string, []T](),
	}
}

func (c *simpleCacheMemory[T]) Delete(key string, inst T) {
	data, ok := c.cache.Get(key)
	if !ok {
		return
	}
	data = utils.RemoveSliceItem(data, inst)
	c.cache.Set(key, data)
	return
}

func (c *simpleCacheMemory[T]) Add(key string, inst T) {
	data, ok := c.cache.Get(key)
	if !ok {
		data = make([]T, 0)
	}
	data = append(data, inst)
	c.cache.Set(key, data)
}

func (c *simpleCacheMemory[T]) ForEach(f func(string, []T)) {
	c.cache.ForEach(func(key string, value []T) bool {
		f(key, value)
		return true
	})
}

func (c *simpleCacheMemory[T]) Close() {}

type simpleCacheItem[T comparable] struct {
	Name  string
	Value T
}
type simpleCacheDB[T comparable] struct {
	save *asyncdb.Save[simpleCacheItem[T]]
}

const (
	IndexSaveSize = 2000
)

func NewSimpleCacheDB[T comparable](
	name string,
	saveSize int,
	save func([]simpleCacheItem[T]),
) *simpleCacheDB[T] {
	if saveSize < IndexSaveSize {
		saveSize = IndexSaveSize // Ensure minimum save size
	}
	return &simpleCacheDB[T]{
		save: asyncdb.NewSave(
			save,
			asyncdb.WithName(name),
			asyncdb.WithSaveSize(saveSize),
			asyncdb.WithSaveTimeout(saveTime),
		),
	}
}

func (c *simpleCacheDB[T]) Delete(key string, inst T) {
	// Implement database deletion logic here
	return
}

func (c *simpleCacheDB[T]) Add(key string, value T) {
	if utils.IsNil(value) {
		return
	}
	c.save.Save(simpleCacheItem[T]{
		Name:  key,
		Value: value,
	})
}

func (c *simpleCacheDB[T]) ForEach(f func(string, []T)) {
	// Implement database iteration logic here
	return
}

func (c *simpleCacheDB[T]) Close() {
	c.save.Close()
}

func NewSimpleCache[T comparable](kind ProgramCacheKind, name string, saveSize int, saveFunc func([]simpleCacheItem[T])) SimpleCache[T] {
	if kind != ProgramCacheMemory {
		return NewSimpleCacheDB[T](name, saveSize, saveFunc)
	} else {
		return NewSimpleCacheMemory[T](name)
	}
}

func (c *ProgramCache) initIndex(databaseKind ProgramCacheKind, saveSize int) {

	c.editorCache = NewSimpleCache[*ssadb.IrSource](
		databaseKind, "EditorCache", saveSize,
		func(iii []simpleCacheItem[*ssadb.IrSource]) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range iii {
					item.Value.Save(tx)
				}
				return nil
			})
			return
		},
	)

	c.offsetCache = NewSimpleCache[*ssadb.IrOffset](
		databaseKind, "OffsetCache", saveSize,
		func(iii []simpleCacheItem[*ssadb.IrOffset]) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range iii {
					ssadb.SaveIrOffset(tx, item.Value)
				}
				return nil
			})
			return
		},
	)

	c.indexCache = NewSimpleCache[*ssadb.IrIndex](
		databaseKind, "IndexCache", saveSize,
		func(iii []simpleCacheItem[*ssadb.IrIndex]) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range iii {
					ssadb.SaveIrIndex(tx, item.Value)
				}
				return nil
			})
			return
		},
	)

	c.VariableIndex = NewSimpleCache[Instruction](
		databaseKind, "VariableIndex", saveSize,
		func(items []simpleCacheItem[Instruction]) {
			for _, item := range items {
				ret := SaveVariableIndexByName(item.Name, item.Value)
				c.indexCache.Add("", ret)

				// save to offset
				if value, ok := item.Value.(Value); ok {
					variable := value.GetVariable(item.Name)
					if !utils.IsNil(c.offsetCache) && !utils.IsNil(variable) {
						for _, offset := range ConvertVariable2Offset(variable, item.Name, int64(value.GetId())) {
							c.offsetCache.Add("", offset)
						}
					}
				}
			}
		},
	)
	c.MemberIndex = NewSimpleCache[Instruction](
		databaseKind, "MemberIndex", saveSize,
		func(items []simpleCacheItem[Instruction]) {
			for _, item := range items {
				item := SaveVariableIndexByMember(item.Name, item.Value)
				c.indexCache.Add("", item)
			}
		},
	)

	c.ClassIndex = NewSimpleCache[Instruction](
		databaseKind, "ClassIndex", saveSize,
		func(items []simpleCacheItem[Instruction]) {
			for _, item := range items {
				item := SaveClassIndex(item.Name, item.Value)
				c.indexCache.Add("", item)
			}
		},
	)

	c.ConstCache = NewSimpleCache[Instruction](
		databaseKind, "ConstCache", saveSize,
		func(ii []simpleCacheItem[Instruction]) {
		},
	)

}
