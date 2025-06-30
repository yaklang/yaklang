package databasex

import (
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type CacheItem[T MemoryItem, D DBItem] struct {
	MemoryItem T
	DBItem     D
}

type Cache[T MemoryItem, D DBItem] struct {
	cache   *DataBaseCacheWithKey[int64, *CacheItem[T, D]]
	fetcher *Fetch[D]
	saver   *Save[D]

	delete func([]D)
}

func NewCache[T MemoryItem, D DBItem](
	ttl time.Duration,

	// marshal
	marshal func(T, D),
	// fetch
	fetch func() []D,

	// delete
	delete func([]D),
	// save and load
	save func([]D),
	load func(int64) (T, D, error),
	opt ...Option,
) *Cache[T, D] {

	fetcher := NewFetch(fetch, opt...)
	saver := NewSave(save, opt...)

	cache := NewDatabaseCacheWithKey[int64, *CacheItem[T, D]](
		ttl,
		func(k int64, v *CacheItem[T, D], reason utils.EvictionReason) bool {
			marshal(v.MemoryItem, v.DBItem)
			saver.Save(v.DBItem)
			return true
		},
		func(i int64) (*CacheItem[T, D], error) {
			t, d, err := load(i)
			if err != nil {
				return nil, utils.Errorf("failed to load item from database")
			}
			return &CacheItem[T, D]{
				MemoryItem: t,
				DBItem:     d,
			}, nil
		},
	)

	c := &Cache[T, D]{
		cache:   cache,
		fetcher: fetcher,
		saver:   saver,
		delete:  delete,
	}
	c.cache.DisableSave()
	return c
}

func (c *Cache[T, U]) Set(item T) {
	if id := item.GetId(); id > 0 {
		if _, ok := c.Get(id); !ok {
			log.Errorf("BUG: Set item with id %d, but not found in cache", id)
		}
		return
	}

	u, err := c.fetcher.Fetch()
	if err != nil {
		return
	}
	id := u.GetIdInt64()
	item.SetId(id)
	c.cache.Set(id, &CacheItem[T, U]{
		MemoryItem: item,
		DBItem:     u,
	})
}

func (c *Cache[T, U]) Get(id int64) (T, bool) {
	item, ok := c.cache.Get(id)
	if !ok {
		return *new(T), false
	}
	return item.MemoryItem, true
}

func (c *Cache[T, D]) Close() {
	c.cache.EnableSave()
	c.cache.Close()
	c.fetcher.Close(c.delete)
	c.saver.Close()
}

func (c *Cache[T, U]) Count() int {
	return c.cache.Count()
}

func (c *Cache[T, D]) GetAll() map[int64]T {
	result := make(map[int64]T, c.cache.Count())
	c.cache.ForEach(func(i int64, ci *CacheItem[T, D]) bool {
		result[i] = ci.MemoryItem
		return true
	})
	return result
}
func (c *Cache[T, D]) Delete(id int64) {
	if c.cache == nil {
		return
	}
}
