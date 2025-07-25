package utils_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// TestUnifiedCacheSingleRequest verifies that a single request correctly loads data.
func TestUnifiedCacheSingleRequest(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "testKey"
	expectedData := "hello_world"
	loadDelay := 10 * time.Millisecond

	loader := func() (string, error) {
		time.Sleep(loadDelay)
		return expectedData, nil
	}

	data, err := cache.GetOrLoad(key, loader)
	if err != nil {
		t.Fatalf("GetOrLoad returned an error: %v", err)
	}
	if data != expectedData {
		t.Errorf("Expected data %q, got %q", expectedData, data)
	}

	// Verify that subsequent immediate call returns cached data without reloading.
	data2, err2 := cache.GetOrLoad(key, loader)
	if err2 != nil {
		t.Fatalf("Second GetOrLoad returned an error: %v", err2)
	}
	if data2 != expectedData {
		t.Errorf("Second call: Expected data %q, got %q", expectedData, data2)
	}

	// Verify that data is accessible via normal Get method
	data3, exists := cache.Get(key)
	if !exists {
		t.Fatal("Data should exist in cache")
	}
	if data3 != expectedData {
		t.Errorf("Get method: Expected data %q, got %q", expectedData, data3)
	}
}

// TestUnifiedCacheConcurrentRequests ensures that multiple concurrent requests
// for the same key trigger the data loader only once and all requests receive the same result.
func TestUnifiedCacheConcurrentRequests(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "concurrentKey"
	expectedData := "concurrent_data"
	loadDelay := 100 * time.Millisecond // Simulate a longer loading time
	loaderCallCount := int32(0)         // Use atomic for concurrent counter

	// DataLoader that counts how many times it's called
	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1) // Increment the counter
		time.Sleep(loadDelay)
		return expectedData, nil
	}

	numRequests := 10
	var wg sync.WaitGroup
	results := make(chan string, numRequests)
	errors := make(chan error, numRequests)

	// Launch multiple goroutines to request the same key
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(requestID int) {
			defer wg.Done()
			data, err := cache.GetOrLoad(key, loader)
			results <- data
			errors <- err
		}(i)
		// Introduce a slight delay to ensure more goroutines hit the "preparing" state
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check all results
	for i := 0; i < numRequests; i++ {
		data := <-results
		err := <-errors
		if err != nil {
			t.Errorf("Request %d: GetOrLoad returned an error: %v", i, err)
			continue
		}
		if data != expectedData {
			t.Errorf("Request: Expected data %q, got %q", expectedData, data)
		}
	}

	// Verify that the data loader was called exactly once
	if count := atomic.LoadInt32(&loaderCallCount); count != 1 {
		t.Errorf("DataLoader was called %d times, expected 1", count)
	}

	// Verify data is in cache
	data, exists := cache.Get(key)
	if !exists {
		t.Fatal("Data should exist in cache after GetOrLoad")
	}
	if data != expectedData {
		t.Errorf("Cache Get: Expected data %q, got %q", expectedData, data)
	}
}

// TestUnifiedCacheErrorHandling verifies that the cache correctly handles
// errors returned by the data loader function.
func TestUnifiedCacheErrorHandling(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "errorKey"
	expectedError := errors.New("loading failed")
	loaderCallCount := int32(0)

	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1)
		return "", expectedError
	}

	// First call should get the error
	data, err := cache.GetOrLoad(key, loader)
	if err == nil {
		t.Fatal("Expected an error from GetOrLoad")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %q, got %q", expectedError.Error(), err.Error())
	}
	if data != "" {
		t.Errorf("Expected empty data on error, got %q", data)
	}

	// Verify that failed load didn't populate the cache
	_, exists := cache.Get(key)
	if exists {
		t.Error("Failed load should not populate cache")
	}

	// Second call should retry the loader (since error wasn't cached)
	_, err2 := cache.GetOrLoad(key, loader)
	if err2 == nil {
		t.Fatal("Expected an error from second GetOrLoad")
	}
	if err2.Error() != expectedError.Error() {
		t.Errorf("Second call: Expected error %q, got %q", expectedError.Error(), err2.Error())
	}

	// Verify that the data loader was called twice (once for each GetOrLoad)
	if count := atomic.LoadInt32(&loaderCallCount); count != 2 {
		t.Errorf("DataLoader was called %d times, expected 2", count)
	}
}

// TestUnifiedCacheMultipleKeys tests that the cache handles different keys independently.
func TestUnifiedCacheMultipleKeys(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key1 := "key1"
	key2 := "key2"
	expectedData1 := "data1"
	expectedData2 := "data2"
	loadDelay := 50 * time.Millisecond

	loader1CallCount := int32(0)
	loader2CallCount := int32(0)

	loader1 := func() (string, error) {
		atomic.AddInt32(&loader1CallCount, 1)
		time.Sleep(loadDelay)
		return expectedData1, nil
	}

	loader2 := func() (string, error) {
		atomic.AddInt32(&loader2CallCount, 1)
		time.Sleep(loadDelay)
		return expectedData2, nil
	}

	var wg sync.WaitGroup
	results := make(chan string, 2)
	keys := make(chan string, 2)

	// Launch goroutines for different keys concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := cache.GetOrLoad(key1, loader1)
		if err != nil {
			t.Errorf("GetOrLoad for key1 failed: %v", err)
			return
		}
		results <- data
		keys <- key1
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		data, err := cache.GetOrLoad(key2, loader2)
		if err != nil {
			t.Errorf("GetOrLoad for key2 failed: %v", err)
			return
		}
		results <- data
		keys <- key2
	}()

	wg.Wait()
	close(results)
	close(keys)

	// Verify results
	receivedData := make(map[string]string)
	for i := 0; i < 2; i++ {
		data := <-results
		key := <-keys
		receivedData[key] = data
	}

	if receivedData[key1] != expectedData1 {
		t.Errorf("Key1: Expected data %q, got %q", expectedData1, receivedData[key1])
	}
	if receivedData[key2] != expectedData2 {
		t.Errorf("Key2: Expected data %q, got %q", expectedData2, receivedData[key2])
	}

	// Verify that each loader was called exactly once
	if count1 := atomic.LoadInt32(&loader1CallCount); count1 != 1 {
		t.Errorf("Loader1 was called %d times, expected 1", count1)
	}
	if count2 := atomic.LoadInt32(&loader2CallCount); count2 != 1 {
		t.Errorf("Loader2 was called %d times, expected 1", count2)
	}
}

// TestUnifiedCacheWithCacheExFeatures tests that cache expiration and other CacheEx features still work
func TestUnifiedCacheWithCacheExFeatures(t *testing.T) {
	shortTTL := 100 * time.Millisecond
	cache := utils.NewTTLCache[string](shortTTL)
	key := "expireKey"
	expectedData := "expire_data"

	loader := func() (string, error) {
		return expectedData, nil
	}

	// Load data
	data, err := cache.GetOrLoad(key, loader)
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if data != expectedData {
		t.Errorf("Expected data %q, got %q", expectedData, data)
	}

	// Verify data is in cache
	cachedData, exists := cache.Get(key)
	if !exists || cachedData != expectedData {
		t.Error("Data should be in cache immediately after load")
	}

	// Wait for expiration
	time.Sleep(shortTTL + 50*time.Millisecond)

	// Verify data has expired
	_, exists = cache.Get(key)
	if exists {
		t.Error("Data should have expired from cache")
	}

	// Test capacity limit
	capacityCache := utils.NewLRUCache[string](2) // Capacity of 2
	capacityCache.Set("item1", "value1")
	capacityCache.Set("item2", "value2")
	capacityCache.Set("item3", "value3") // Should evict item1

	_, exists = capacityCache.Get("item1")
	if exists {
		t.Error("item1 should have been evicted")
	}
	_, exists = capacityCache.Get("item3")
	if !exists {
		t.Error("item3 should exist")
	}
}
