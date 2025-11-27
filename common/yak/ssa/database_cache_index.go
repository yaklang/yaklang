package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asyncdb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type simpleCacheItem[T comparable] struct {
	Name  string
	Value T
}

type SimpleCache[T comparable] struct {
	name  string
	cache *utils.SafeMapWithKey[string, []T] // if  memory exist
	save  *asyncdb.Save[simpleCacheItem[T]]  // save to database
}

func NewSimpleCache[T comparable](name string) *SimpleCache[T] {
	return &SimpleCache[T]{
		name:  name,
		cache: utils.NewSafeMapWithKey[string, []T](),
	}
}

func (c *SimpleCache[T]) Delete(key string, inst T) {
	data, ok := c.cache.Get(key)
	if !ok {
		return
	}
	data = utils.RemoveSliceItem(data, inst)
	c.cache.Set(key, data)
	return
}

func (c *SimpleCache[T]) Add(key string, item T) {
	if utils.IsNil(item) {
		return
	}

	data, ok := c.cache.Get(key)
	if !ok {
		data = make([]T, 0)
	}
	data = append(data, item)
	c.cache.Set(key, data)
	if c.save != nil {
		c.save.Save(simpleCacheItem[T]{
			Name:  key,
			Value: item,
		})
	}
}

func (c *SimpleCache[T]) ForEach(f func(string, []T)) {
	c.cache.ForEach(func(key string, value []T) bool {
		f(key, value)
		return true
	})
}

func (c *SimpleCache[T]) Close() {
	if c.save != nil {
		c.save.Close()
	}
}

const (
	IndexSaveSize = 2000
)

func (s *SimpleCache[T]) SetSaver(f func([]simpleCacheItem[T]), opt ...asyncdb.Option) {
	opt = append(opt,
		asyncdb.WithSaveSize(defaultSaveSize),
		asyncdb.WithSaveTimeout(saveTime),
	)
	s.save = asyncdb.NewSave(f, opt...)
}

func (c *ProgramCache) initIndex(databaseKind ProgramCacheKind, saveSize int) {
	if saveSize < IndexSaveSize {
		saveSize = IndexSaveSize // Ensure minimum save size
	}
	c.editorCache = NewSimpleCache[*ssadb.IrSource]("EditorCache")
	if databaseKind == ProgramCacheDBWrite {
		c.editorCache.SetSaver(
			func(iii []simpleCacheItem[*ssadb.IrSource]) {
				saveStep := func() error {
					return utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
						for _, item := range iii {
							if err := tx.Save(item.Value).Error; err != nil {
								log.Errorf("DATABASE: save ir source to database error: %v", err)
							}
						}
						return nil
					})
				}
				c.diagnosticsTrack("ssa.Database.SaveIrSourceBatch", saveStep)
				return
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

	c.offsetCache = NewSimpleCache[*ssadb.IrOffset]("OffsetCache")
	if databaseKind == ProgramCacheDBWrite {
		c.offsetCache.SetSaver(
			func(iii []simpleCacheItem[*ssadb.IrOffset]) {
				saveStep := func() error {
					return utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
						for _, item := range iii {
							ssadb.SaveIrOffset(tx, item.Value)
						}
						return nil
					})
				}
				c.diagnosticsTrack("ssa.Database.SaveIrOffsetBatch", saveStep)
				return
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

	c.indexCache = NewSimpleCache[*ssadb.IrIndex]("IndexCache")
	if databaseKind == ProgramCacheDBWrite {
		c.indexCache.SetSaver(
			func(iii []simpleCacheItem[*ssadb.IrIndex]) {
				saveStep := func() error {
					return utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
						for _, item := range iii {
							ssadb.SaveIrIndex(tx, item.Value)
						}
						return nil
					})
				}
				c.diagnosticsTrack("ssa.Database.SaveIrIndexBatch", saveStep)
				return
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

	c.VariableIndex = NewSimpleCache[Instruction]("VariableIndex")
	if databaseKind == ProgramCacheDBWrite {
		c.VariableIndex.SetSaver(
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
			asyncdb.WithSaveSize(saveSize),
		)
	}
	c.MemberIndex = NewSimpleCache[Instruction]("MemberIndex")
	if databaseKind == ProgramCacheDBWrite {
		c.MemberIndex.SetSaver(
			func(items []simpleCacheItem[Instruction]) {
				for _, item := range items {
					item := SaveVariableIndexByMember(item.Name, item.Value)
					c.indexCache.Add("", item)
				}
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

	c.ClassIndex = NewSimpleCache[Instruction]("ClassIndex")
	if databaseKind == ProgramCacheDBWrite {
		c.ClassIndex.SetSaver(
			func(items []simpleCacheItem[Instruction]) {
				for _, item := range items {
					item := SaveClassIndex(item.Name, item.Value)
					c.indexCache.Add("", item)
				}
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

	c.ConstCache = NewSimpleCache[Instruction]("ConstCache")
	if databaseKind == ProgramCacheDBWrite {
		c.ConstCache.SetSaver(
			func(ii []simpleCacheItem[Instruction]) {
			},
			asyncdb.WithSaveSize(saveSize),
		)
	}

}
