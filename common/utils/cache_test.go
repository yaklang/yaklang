package utils

import (
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
