package asyncdb

import (
	"sync"
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
	config  *config

	delete DeleteFunc[D]
}

func NewCache[T MemoryItem, D DBItem](
	ttl time.Duration,

	// marshal
	marshal MarshalFunc[T, D],
	// fetch
	fetch FetchFunc[D],

	// delete
	delete DeleteFunc[D],
	// save and load
	save SaveFunc[D],
	load LoadFunc[T, D],
	opt ...Option,
) *Cache[T, D] {
	config := NewConfig(opt...)
	fetcher := NewFetchWithConfig(fetch, config)
	saver := NewSaveWithConfig(save, config)

	cache := NewDatabaseCacheWithKey[int64, *CacheItem[T, D]](
		ttl,
		func(k int64, v *CacheItem[T, D], reason utils.EvictionReason) bool {
			// if not marshal function is set, disable this function
			if utils.IsNil(marshal) {
				log.Errorf("BUG: marshal function is not set")
				return false
			}
			marshal(v.MemoryItem, v.DBItem)
			saver.Save(v.DBItem)
			return true
		},
		func(i int64) (*CacheItem[T, D], error) {
			// if not set load, disable this function
			if utils.IsNil(load) {
				return nil, utils.Errorf("load function is not set")
			}

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
		config:  config,
	}
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
	var item *CacheItem[T, U]
	var ok bool
	item, ok = c.cache.Get(id)
	if !ok {
		return *new(T), false
	}
	return item.MemoryItem, true
}

func (c *Cache[T, D]) Close(wgs ...*sync.WaitGroup) {
	if c.fetcher == nil {
		return
	}

	var wg *sync.WaitGroup
	if len(wgs) > 0 {
		wg = wgs[0]
	} else {
		wg = &sync.WaitGroup{}
		defer wg.Wait()
	}

	if !utils.IsNil(c.delete) {
		wg.Add(1)
		go func() {
			wg.Done()
			c.fetcher.DeleteRest(c.delete, wg)
		}()
	}

	c.cache.EnableSave()
	c.cache.Close()

	if !utils.IsNil(c.saver) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.saver.Close()
		}()
	}

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
	c.cache.Delete(id)
}
