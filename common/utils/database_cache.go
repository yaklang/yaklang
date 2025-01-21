package utils

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type databaseCacheItemStatus int

const (
	DatabaseCacheItemNormal databaseCacheItemStatus = iota // save to database before  expired
	DatabaseCacheItemSave                                  // expired, save to database and delete(default) or update(Update-status)
	DatabaseCacheItemUpdate                                // someone peek the instruction when save, update the instruction after save
	DatabaseCacheItemNotFound
)

// T: memory data type
type databaseCacheItem[K comparable, T any] struct {
	status     databaseCacheItemStatus
	key        K
	memoryItem T
}

// save to database
// attention: this function should be blocking
type SaveDatabase[K comparable, T any] func(K, T, EvictionReason) bool

// load data from database by key
// attention: this function should be blocking
type LoadFromDatabase[K comparable, T any] func(K) (T, error)

type DataBaseCacheWithKey[K comparable, T any] struct {
	notifyCache *CacheExWithKey[string, K]
	data        *SafeMapWithKey[K, databaseCacheItem[K, T]]

	saveDatabase     SaveDatabase[K, T]
	loadFromDatabase LoadFromDatabase[K, T]
	wait             *sync.WaitGroup
}

/*
param:
cache : ttl/lru cache
save: save to database, function should be blocking, return true is success
load: load from database, function should be blocking, return data and error
*/
func NewDatabaseCacheWithKey[K comparable, T any](
	ttl time.Duration,
	save SaveDatabase[K, T],
	load LoadFromDatabase[K, T],
) *DataBaseCacheWithKey[K, T] {
	cache := NewCacheExWithKey[string, K](WithCacheTTL(ttl))
	ret := &DataBaseCacheWithKey[K, T]{
		notifyCache:      cache,
		data:             NewSafeMapWithKey[K, databaseCacheItem[K, T]](),
		saveDatabase:     save,
		loadFromDatabase: load,
		wait:             &sync.WaitGroup{},
	}
	cache.SetExpirationCallback(func(_ string, key K, reason EvictionReason) {
		log.Debugf("expire key: %v", key)
		ret.save(key, reason)
	})
	return ret
}

func (c *DataBaseCacheWithKey[K, T]) updateStatus(item databaseCacheItem[K, T], status databaseCacheItemStatus) {
	log.Debugf("update status, key: %v, status: %v", item.key, status)
	item.status = status
	c.data.Set(item.key, item)
}

func GetDatabaseCacheStatus[K comparable, T any](c *DataBaseCacheWithKey[K, T], key K) databaseCacheItemStatus {
	if item, ok := c.data.Get(key); ok {
		return item.status
	}
	return DatabaseCacheItemNotFound
}

func (c *DataBaseCacheWithKey[K, T]) Set(key K, memValue T) {
	if item, ok := c.data.Get(key); ok {
		_ = item
		// already exist
		log.Errorf("BUG:: already exist in cache, key: %v", key)
		return
	}
	c.notifyCache.Set(InterfaceToString(key), key)
	c.data.Set(key, databaseCacheItem[K, T]{
		status:     DatabaseCacheItemNormal,
		key:        key,
		memoryItem: memValue,
	})
}

func (c *DataBaseCacheWithKey[K, T]) Get(key K) (T, bool) {
	// get from cache
	if item, ok := c.data.Get(key); ok {
		if item.status == DatabaseCacheItemSave {
			// if this item is saving to database, this item need update to normal after save
			c.updateStatus(item, DatabaseCacheItemUpdate)
		}
		// return memory data
		return item.memoryItem, true
	}

	// no in cache, load from database
	if memValue, err := c.loadFromDatabase(key); err == nil {
		if item, ok := c.data.Get(key); ok {
			return item.memoryItem, true
		}
		c.Set(key, memValue)
		return memValue, true
	}

	var zero T
	return zero, false
}

func (c *DataBaseCacheWithKey[K, T]) GetAll() map[K]T {
	ret := make(map[K]T)
	c.data.ForEach(func(key K, value databaseCacheItem[K, T]) bool {
		ret[key] = value.memoryItem
		return true
	})
	return ret
}

func (c *DataBaseCacheWithKey[K, T]) save(key K, reason EvictionReason) {
	// in goroutine
	item, ok := c.data.Get(key)
	if !ok {
		// no this item
		log.Errorf("BUG:: no this item in cache, key: %v", key)
		return
	}

	recoverData := func() {
		// recover c.notifyCache
		c.notifyCache.Set(InterfaceToString(key), key)
		c.updateStatus(item, DatabaseCacheItemNormal)
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("save failed: %v", err)
			PrintCurrentGoroutineRuntimeStack()

			// recover data
			recoverData()
		}
	}()

	// update to save
	c.updateStatus(item, DatabaseCacheItemSave)

	// save to database
	save_success := c.saveDatabase(item.key, item.memoryItem, reason) // wait this

	// check status
	item, ok = c.data.Get(key)
	if !ok {
		// no this item
		log.Errorf("BUG:: no this item in cache, key: %v", key)
		return
	}
	if save_success {
		switch item.status {
		case DatabaseCacheItemSave:
			// normal save to database and no one care, just delete this item
			// c.notifyCache deleted this item already
			c.data.Delete(key)
		case DatabaseCacheItemUpdate:
			// someone peek the item when saving, update the item now
			recoverData()
		case DatabaseCacheItemNormal:
			// not run here !
			log.Errorf("BUG:: after save item status is Normal, key: %v", key)
		}
	} else {
		recoverData()
	}
}

func (c *DataBaseCacheWithKey[K, T]) Delete(key K) {
	c.notifyCache.Delete(InterfaceToString(key))
}

func (c *DataBaseCacheWithKey[K, T]) Count() int {
	return c.data.Count()
}

func (c *DataBaseCacheWithKey[K, T]) Close() {
	// todo: save all item
	// return c.data.Close()
	c.notifyCache.Close()
}

func (c *DataBaseCacheWithKey[K, T]) ForEach(f func(K, T) bool) {
	c.data.ForEach(func(key K, value databaseCacheItem[K, T]) bool {
		return f(key, value.memoryItem)
	})
}
