package asyncdb_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/asyncdb"
)

func TestDatabaseCache(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	_ = database
	ttl := time.Millisecond * 100

	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		ttl,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			database.Set(i, s)
			return true
		},
		func(i int) (string, error) {
			log.Infof("load from database, key: %v", i)
			if value, ok := database.Get(i); ok {
				return value, nil
			} else {
				return "", utils.Errorf("no this id in database ")
			}
		},
	)

	// set data
	cache.Set(1, "1")
	cache.Set(2, "2")

	time.Sleep(2 * ttl)
	cache.Set(3, "3") // 1, 2 will be saved to database

	// check 1 save to database
	data1DB, ok := database.Get(1)
	require.True(t, ok)
	require.Contains(t, "1", data1DB)

	// check 2 save to database
	data2DB, ok := database.Get(2)
	require.True(t, ok)
	require.Contains(t, "2", data2DB)

	data1, ok := cache.Get(1) // load from database
	// check get 1 is ok
	require.True(t, ok)
	require.Equal(t, "1", data1)
}

func TestDatabaseCache_WithDatabaseTime(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	_ = database
	ttl := time.Millisecond * 100
	databaseTime := ttl

	load := utils.NewSafeMapWithKey[int, struct{}]()

	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		ttl,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			database.Set(i, s)
			//  database need time
			time.Sleep(databaseTime)
			return true
		},
		func(i int) (string, error) {
			load.Set(i, struct{}{})
			log.Infof("load from database, key: %v", i)
			// database need time
			time.Sleep(databaseTime)
			if value, ok := database.Get(i); ok {
				return value, nil
			} else {
				return "", utils.Errorf("no this id in database ")
			}
		},
	)

	cache.Set(1, "1")
	cache.Set(2, "2")
	// wait for 1, 2 save to database
	time.Sleep(ttl + 10*time.Millisecond)

	// now 1, 2 in database saving status
	// get 1
	data, ok := cache.Get(1) // 1 status will set to update
	require.True(t, ok)
	require.Equal(t, "1", data)
	// 1 will not delete from cache

	// wait save finish
	time.Sleep(databaseTime)

	// get 2
	data, ok = cache.Get(2)
	require.True(t, ok)
	require.Equal(t, "2", data)

	// check load
	_, ok = load.Get(1)
	require.False(t, ok)
	_, ok = load.Get(2)
	require.True(t, ok)
}

func TestDatabaseCache_ManualDelete(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	_ = database
	ttl := time.Millisecond * 100
	log.SetLevel(log.DebugLevel)

	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		time.Second*10,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			database.Set(i, s)
			return true
		},
		func(i int) (string, error) {
			log.Infof("load from database, key: %v", i)
			if value, ok := database.Get(i); ok {
				return value, nil
			} else {
				return "", utils.Errorf("no this id in database ")
			}
		},
	)

	// set data
	cache.Set(1, "1")
	cache.Set(2, "2")

	cache.Delete(1)
	time.Sleep(2 * ttl) // wait save

	// check 1 save to database
	data1DB, ok := database.Get(1)
	require.True(t, ok)
	require.Contains(t, "1", data1DB)

	data1, ok := cache.Get(1) // load from database
	// check get 1 is ok
	require.True(t, ok)
	require.Equal(t, "1", data1)
}

func TestDatabaseCache_NoDatabase(t *testing.T) {
	// just in memory
	ttl := time.Millisecond * 100
	log.SetLevel(log.DebugLevel)
	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		ttl,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			return false // no save to database
		},
		func(i int) (string, error) {
			return "", utils.Errorf("no this id in database ")
		},
	)

	cache.Set(1, "1")
	cache.Set(2, "2")
	// wait for 1, 2 save to database
	time.Sleep(ttl + 10*time.Millisecond)

	// now 1, 2 still in cache
	// get 1
	data, ok := cache.Get(1) // 1 status will set to update
	require.True(t, ok)
	require.Equal(t, "1", data)
	// 1 will not delete from cache
	// get 2
	data, ok = cache.Get(2)
	require.True(t, ok)
	require.Equal(t, "2", data)
}

func TestDatabaseCache_Close(t *testing.T) {
	database := utils.NewSafeMapWithKey[int, string]()
	_ = database
	ttl := time.Millisecond * 100

	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		ttl,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			database.Set(i, s)
			return true
		},
		func(i int) (string, error) {
			log.Infof("load from database, key: %v", i)
			if value, ok := database.Get(i); ok {
				return value, nil
			} else {
				return "", utils.Errorf("no this id in database ")
			}
		},
	)

	// set data
	cache.Set(1, "1")
	cache.Set(2, "2")

	cache.Close()

	// check 1 save to database
	data1DB, ok := database.Get(1)
	require.True(t, ok)
	require.Contains(t, "1", data1DB)

	// check 2 save to database
	data2DB, ok := database.Get(2)
	require.True(t, ok)
	require.Contains(t, "2", data2DB)
}
func TestDatabaseCache_DisableEnableSave(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	database := utils.NewSafeMapWithKey[int, string]()
	ttl := time.Millisecond * 100

	cache := asyncdb.NewDatabaseCacheWithKey[int, string](
		ttl,
		func(i int, s string, reason utils.EvictionReason) bool {
			log.Infof("save to database, key: %v, value: %v", i, s)
			database.Set(i, s)
			return true
		},
		func(i int) (string, error) {
			log.Infof("load from database, key: %v", i)
			if value, ok := database.Get(i); ok {
				return value, nil
			} else {
				return "", utils.Errorf("no this id in database ")
			}
		},
	)

	// Disable saving to database
	cache.DisableSave()
	require.True(t, cache.IsSaveDisabled())

	// Set data while saving is disabled
	cache.Set(1, "1")
	cache.Set(2, "2")

	// Wait for TTL to expire
	time.Sleep(2 * ttl)

	// Check that data was not saved to database
	_, ok := database.Get(1)
	require.False(t, ok, "Data should not be saved to database when saving is disabled")
	_, ok = database.Get(2)
	require.False(t, ok, "Data should not be saved to database when saving is disabled")

	// Data should still be in cache
	data1, ok := cache.Get(1)
	require.True(t, ok)
	require.Equal(t, "1", data1)

	// Enable saving to database
	cache.EnableSave()
	require.False(t, cache.IsSaveDisabled())

	// Set new data
	cache.Set(3, "3")

	// Wait for TTL to expire
	time.Sleep(2 * ttl)

	// Check that new data was saved to database
	data3DB, ok := database.Get(3)
	require.True(t, ok)
	require.Equal(t, "3", data3DB)

	// Old data should still be in cache since it was recovered when save was disabled
	data1, ok = cache.Get(1)
	require.True(t, ok)
	require.Equal(t, "1", data1)
}
