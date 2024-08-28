package utils

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	c := NewTTLCache[int](10 * time.Minute)

	isNewItemCallback := false
	expirationCallbackCount, wantExpirationCallbackCount := 0, 2
	isCheckExpirationCallback := false
	hasResetTTL := false
	c.SetNewItemCallback(func(key string, value int) {
		isNewItemCallback = true
	})
	c.SetExpirationCallback(func(key string, value int) {
		expirationCallbackCount++
	})
	c.SetCheckExpirationCallback(func(key string, value int) bool {
		isCheckExpirationCallback = true
		if key == "four" && !hasResetTTL {
			hasResetTTL = true
			return false
		}
		return true
	})
	c.SkipTtlExtensionOnHit(true)

	c.SetWithTTL("one", 1, 1*time.Second)
	c.SetWithTTL("four", 4, 1*time.Second)
	if v, ok := c.Get("one"); !ok || v != 1 {
		t.Fatal("TTLCache get/set failed")
	}
	// 1.5s:
	time.Sleep(1500 * time.Millisecond)
	if _, ok := c.Get("one"); ok {
		t.Fatal("TTLCache live time failed")
	}
	if v, ok := c.Get("four"); !ok || v != 4 {
		t.Fatal("TTLCache SetCheckExpirationCallback failed, want reset ttl, but not")
	}

	c.Set("two", 2)
	c.Set("three", 3)
	all := c.GetAll()
	if len(all) != 3 {
		t.Fatalf("TTLCache GetAll failed: number want 3 but got %d", len(all))
	}
	for _, v := range c.GetAll() {
		if v != 2 && v != 3 && v != 4 {
			t.Fatalf("TTLCache GetAll failed: want 2/3/4 but got %d", v)
		}
	}

	// 2s: test skip reset TTL
	c.Get("four")
	time.Sleep(1000 * time.Millisecond)
	if _, ok := c.Get("four"); ok {
		t.Fatal("TTLCache SkipTtlExtensionOnHit failed, want disable reset ttl, but not")
	}

	c.Purge()
	all = c.GetAll()
	if len(all) != 0 {
		t.Fatalf("TTLCache Purge failed: want size = 0 but got %d", len(all))
	}

	if !isNewItemCallback {
		t.Fatal("TTLCache SetNewItemCallback failed, want callback SetNewItemCallback but not")
	}

	if expirationCallbackCount != wantExpirationCallbackCount {
		t.Fatalf("TTLCache SetExpirationCallback failed, want callback SetExpirationCallback %d time but got %d", wantExpirationCallbackCount, expirationCallbackCount)
	}
	if !isCheckExpirationCallback {
		t.Fatal("TTLCache SetCheckExpirationCallback failed, want callback SetCheckExpirationCallback but not")
	}
}

func TestTTLCacheConcurrency(t *testing.T) {
	// 初始化一个具有10分钟过期时间的TTLCache
	c := NewTTLCache[int](10 * time.Minute)
	var wg sync.WaitGroup

	// 并发执行的goroutines数量
	numGoroutines := 50
	// 每个goroutine执行的操作次数
	numOperationsPerGoroutine := 100

	// 使用互斥锁和错误切片来记录并发过程中产生的错误
	var mu sync.Mutex
	errors := make([]error, 0)

	// recordError函数用于安全地记录错误
	recordError := func(err error) {
		mu.Lock()
		errors = append(errors, err)
		mu.Unlock()
	}

	// 启动多个goroutines进行并发测试
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := id * j

				// 测试并发写入
				c.Set(key, value)

				// 测试并发读取
				if v, ok := c.Get(key); !ok || v != value {
					recordError(fmt.Errorf("mismatched %s value: expected %d, got %d", key, value, v))
				}

				// 每隔30次操作，调用Purge方法清空缓存
				//if j%30 == 0 {
				//	c.Purge()
				//}
			}
		}(i)
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 检查是否有错误发生
	if len(errors) > 0 {
		for _, err := range errors {
			t.Error(err)
		}
	}
}

func TestTTLCacheGetTTLExtension(t *testing.T) {
	// 设置一个较短的过期时间
	ttl := 2 * time.Second
	c := NewTTLCache[int](ttl)
	key := "activeKey"
	value := 42

	// 设置过期回调以监控键的过期
	var expired bool
	c.SetExpirationCallback(func(k string, v int) {
		if k == key {
			expired = true
		}
	})

	// 设置键值对
	c.Set(key, value)
	time.Sleep(1 * time.Second)
	// 启动多个 goroutine 来频繁访问键
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if v, ok := c.Get(key); !ok || v != value {
					t.Errorf("Expected value %d for key '%s', got %d", value, key, v)
				}
				time.Sleep(500 * time.Millisecond) // 访问间隔小于TTL
			}
		}()
	}

	wg.Wait()

	// 给足够的时间让键过期
	time.Sleep(1900 * time.Microsecond)

	// 检查键是否因为频繁访问而未过期
	if expired {
		t.Errorf("Key '%s' expired despite frequent accesses", key)
	} else {
		t.Logf("Key '%s' did not expire as expected due to frequent accesses", key)
	}
}
