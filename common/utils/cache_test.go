package utils

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestTTLCache(t *testing.T) {
	c := NewTTLCache[int](10 * time.Minute)

	isNewItemCallback := false
	expirationCallbackCount, wantExpirationCallbackCount := uint64(0), uint64(2)
	c.SetNewItemCallback(func(key string, value int) {
		isNewItemCallback = true
	})
	c.SetExpirationCallback(func(key string, value int) {
		t.Logf("expiration callback: %s, %d", key, value)
		atomic.AddUint64(&expirationCallbackCount, 1)
	})

	c.SetWithTTL("one", 1, 1*time.Second)
	c.SetWithTTL("four", 4, 1*time.Second)
	if v, ok := c.Get("one"); !ok || v != 1 {
		t.Fatal("TTLCache get/set failed")
	}
	time.Sleep(1500 * time.Millisecond) // 1.5s
	if _, ok := c.Get("one"); ok {
		t.Fatal("TTLCache live time failed")
	}
	if _, ok := c.Get("four"); ok {
		t.Fatal("TTLCache live time failed")
	}

	c.Set("two", 2)
	c.Set("three", 3)
	all := c.GetAll()
	if len(all) != 2 {
		t.Fatalf("TTLCache GetAll failed: number want 3 but got %d", len(all))
	}
	for _, v := range c.GetAll() {
		if v != 2 && v != 3 {
			t.Fatalf("TTLCache GetAll failed: want 2/3/4 but got %d", v)
		}
	}

	// 2s: test skip reset TTL
	c.SkipTtlExtensionOnHit(true)
	time.Sleep(500 * time.Millisecond)
	c.Get("four")
	time.Sleep(600 * time.Millisecond)
	// after 1.1s, four should be expired
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
}

func TestLRUCache(t *testing.T) {
	c := NewLRUCache[int](2)

	isNewItemCallback := false
	capacityReachCallbackCount := uint64(0)
	c.SetNewItemCallback(func(key string, value int) {
		t.Logf("new item callback: %s", key)
		isNewItemCallback = true
	})
	c.SetExpirationCallback(func(key string, value int) {
		t.Logf("capacity reach callback: %s, %d", key, value)
		atomic.AddUint64(&capacityReachCallbackCount, 1)
	})

	c.Set("one", 1)                           // 1
	c.Set("two", 2)                           // 2 1
	if v, ok := c.Get("one"); !ok || v != 1 { // 1 2
		t.Fatal("LRUCache get/set failed")
	}
	c.Set("three", 3) // 3 1 2(delete)
	if _, ok := c.Get("two"); ok {
		t.Fatal("LRUCache capacity limit failed")
	}
	if v, ok := c.Get("one"); !ok || v != 1 {
		t.Fatal("LRUCache eviction policy failed")
	}

	all := c.GetAll() // 3 1
	if len(all) != 2 {
		t.Fatalf("LRUCache GetAll failed: number want 2 but got %d", len(all))
	}
	for _, v := range c.GetAll() {
		if v != 1 && v != 3 {
			t.Fatalf("LRUCache GetAll failed: want 1/3 but got %d", v)
		}
	}

	c.Purge()
	all = c.GetAll()
	if len(all) != 0 {
		t.Fatalf("LRUCache Purge failed: want size = 0 but got %d", len(all))
	}

	time.Sleep(1 * time.Second) // wait all callback done

	if !isNewItemCallback {
		t.Fatal("LRUCache SetNewItemCallback failed, want callback SetNewItemCallback but not")
	}

	if count := atomic.LoadUint64(&capacityReachCallbackCount); count != 1 {
		t.Fatalf("LRUCache SetCapacityReachCallback failed, want callback SetCapacityReachCallback 1 time but got %d", count)
	}
}

func TestCombinedCache(t *testing.T) {
	c := NewCacheEx[int](WithCacheCapacity(2), WithCacheTTL(10*time.Minute))

	isNewItemCallback := false
	expirationCallbackCount := uint64(0)
	capacityReachCallbackCount := uint64(0)
	c.SetNewItemCallback(func(key string, value int) {
		isNewItemCallback = true
	})
	c.SetExpirationCallback(func(key string, value int, reason EvictionReason) {
		switch reason {
		case EvictionReasonCapacityReached:
			t.Logf("capacity reach callback: %s, %d", key, value)
			atomic.AddUint64(&capacityReachCallbackCount, 1)
		case EvictionReasonExpired:
			t.Logf("expiration callback: %s, %d", key, value)
			atomic.AddUint64(&expirationCallbackCount, 1)
		}
	})

	c.SetWithTTL("one", 1, 1*time.Second)     // 1
	c.SetWithTTL("four", 4, 1*time.Second)    // 4 1
	if v, ok := c.Get("one"); !ok || v != 1 { // 1 4
		t.Fatal("CombinedCache get/set failed")
	}
	time.Sleep(1500 * time.Millisecond) // 1.5 * second // expiration 1 and 4
	if _, ok := c.Get("one"); ok {      // 1 expiration
		t.Fatal("CombinedCache TTL live time failed")
	}
	if _, ok := c.Get("four"); ok { // 4 expiration
		t.Fatal("CombinedCache SetCheckExpirationCallback failed, want reset ttl, but not")
	}

	c.SetWithTTL("one", 1, 1*time.Second) // 1
	time.Sleep(600 * time.Millisecond)    // 0.6s
	c.Set("four", 4)                      // 4 1
	time.Sleep(600 * time.Millisecond)    // 1.2s // 1 expiration
	if _, ok := c.Get("one"); ok {
		t.Fatal("CombinedCache TTL live time failed")
	}
	// 4

	c.Set("two", 2)   // 2 4
	c.Set("three", 3) // 3 2 4(delete)

	t.Logf("lru cache : %v", c.GetAll()) // 3 2
	if v, ok := c.Get("two"); !ok || v != 2 {
		t.Fatal("CombinedCache LRU capacity limit failed")
	}
	if _, ok := c.Get("four"); ok {
		t.Fatal("CombinedCache eviction policy failed")
	}

	all := c.GetAll() // 2 3
	if len(all) != 2 {
		t.Fatalf("CombinedCache GetAll failed: number want 2 but got %d", len(all))
	}
	for _, v := range c.GetAll() {
		if v != 3 && v != 2 {
			t.Fatalf("CombinedCache GetAll failed: want 3/4 but got %d", v)
		}
	}

	c.Purge()
	all = c.GetAll()
	if len(all) != 0 {
		t.Fatalf("CombinedCache Purge failed: want size = 0 but got %d", len(all))
	}

	time.Sleep(1 * time.Second) // wait all callback done

	if !isNewItemCallback {
		t.Fatal("CombinedCache SetNewItemCallback failed, want callback SetNewItemCallback but not")
	}

	if expirationCallbackCount != 3 {
		t.Fatalf("CombinedCache SetExpirationCallback failed, want callback SetExpirationCallback %d time but got %d", 3, expirationCallbackCount)
	}

	if capacityReachCallbackCount != 1 {
		t.Fatalf("CombinedCache SetCapacityReachCallback failed, want callback SetCapacityReachCallback 1 time but got %d", capacityReachCallbackCount)
	}

}
