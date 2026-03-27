package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type simpleCacheItem[T comparable] struct {
	Name  string
	Value T
}

type SimpleCache[T comparable] struct {
	name  string
	cache *utils.SafeMapWithKey[string, []T] // if  memory exist
	save  *dbcache.Save[simpleCacheItem[T]]  // save to database
}

func NewSimpleCache[T comparable](name string, keepResident ...bool) *SimpleCache[T] {
	resident := true
	if len(keepResident) > 0 {
		resident = keepResident[0]
	}
	var cache *utils.SafeMapWithKey[string, []T]
	if resident {
		cache = utils.NewSafeMapWithKey[string, []T]()
	}
	return &SimpleCache[T]{
		name:  name,
		cache: cache,
	}
}

func (c *SimpleCache[T]) Delete(key string, inst T) {
	if c == nil || c.cache == nil {
		return
	}
	data, ok := c.cache.Get(key)
	if !ok {
		return
	}
	data = utils.RemoveSliceItem(data, inst)
	c.cache.Set(key, data)
	return
}

func (c *SimpleCache[T]) Add(key string, item T) {
	if c == nil || utils.IsNil(item) {
		return
	}
	if c.cache != nil {
		data, ok := c.cache.Get(key)
		if !ok {
			data = make([]T, 0, 1)
		} else if len(data) > 0 && data[len(data)-1] == item {
			return
		}
		data = append(data, item)
		c.cache.Set(key, data)
	}
	if c.save != nil {
		c.save.Save(simpleCacheItem[T]{
			Name:  key,
			Value: item,
		})
	}
}

func (c *SimpleCache[T]) Has(key string) bool {
	if c == nil || c.cache == nil {
		return false
	}
	_, ok := c.cache.Get(key)
	return ok
}

func (c *SimpleCache[T]) ForEach(f func(string, []T)) {
	if c == nil || c.cache == nil {
		return
	}
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

func (s *SimpleCache[T]) SetSaver(f func([]simpleCacheItem[T]), opt ...dbcache.Option) {
	baseOpt := []dbcache.Option{
		dbcache.WithSaveSize(defaultSaveSize),
		dbcache.WithSaveTimeout(saveTime),
	}
	baseOpt = append(baseOpt, opt...)
	s.save = dbcache.NewSave(f, baseOpt...)
}

func (c *ProgramCache) initIndex(cfg *ssaconfig.Config, databaseKind ProgramCacheKind, saveSize int) {
	saveSize = resolveAuxiliarySaveSize(cfg, saveSize)
	c.editorHashCache = utils.NewSafeMapWithKey[string, struct{}]()
	c.editorCache = NewSimpleCache[*ssadb.IrSource]("EditorCache", false)
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
			dbcache.WithSaveSize(saveSize),
		)
	}

	c.offsetCache = NewSimpleCache[*ssadb.IrOffset]("OffsetCache", false)
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
			dbcache.WithSaveSize(saveSize),
		)
	}

	c.indexCache = NewSimpleCache[*ssadb.IrIndex]("IndexCache", false)
	if databaseKind == ProgramCacheDBWrite {
		c.indexCache.SetSaver(
			func(iii []simpleCacheItem[*ssadb.IrIndex]) {
				saveStep := func() error {
					return utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
						var indices []*ssadb.IrIndex
						for _, item := range iii {
							if item.Value != nil {
								indices = append(indices, item.Value)
							}
						}
						ssadb.SaveIrIndexBatch(tx, indices)
						return nil
					})
				}
				c.diagnosticsTrack("ssa.Database.SaveIrIndexBatch", saveStep)
				return
			},
			dbcache.WithSaveSize(saveSize),
		)
	}

	c.VariableIndex = NewSimpleCache[int64]("VariableIndex")
	c.MemberIndex = NewSimpleCache[int64]("MemberIndex")
	c.ClassIndex = NewSimpleCache[int64]("ClassIndex")
	c.ConstCache = NewSimpleCache[int64]("ConstCache")
}

func (c *ProgramCache) enqueueVariableIndex(name string, inst Instruction) {
	if c == nil || c.ProgramCacheKind != ProgramCacheDBWrite || utils.IsNil(inst) {
		return
	}

	ret := CreateVariableIndexByName(name, inst)
	c.indexCache.Add("", ret)

	value, ok := inst.(Value)
	if !ok {
		return
	}
	variable := value.GetVariable(name)
	if utils.IsNil(variable) {
		return
	}
	for _, offset := range ConvertVariable2Offset(variable, name, value.GetId()) {
		c.offsetCache.Add("", offset)
	}
}

func (c *ProgramCache) enqueueMemberIndex(name string, inst Instruction) {
	if c == nil || c.ProgramCacheKind != ProgramCacheDBWrite || utils.IsNil(inst) {
		return
	}
	c.indexCache.Add("", CreateVariableIndexByMember(name, inst))
}

func (c *ProgramCache) enqueueClassIndex(name string, inst Instruction) {
	if c == nil || c.ProgramCacheKind != ProgramCacheDBWrite || utils.IsNil(inst) {
		return
	}
	c.indexCache.Add("", CreateClassIndex(name, inst))
}
