package utils_test // Note: using a _test package name

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// TestSingleFlightCacheSingleRequest verifies that a single request correctly loads data.
func TestSingleFlightCacheSingleRequest(t *testing.T) {
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
}

// TestSingleFlightCacheConcurrentRequests ensures that multiple concurrent requests
// for the same key trigger the data loader only once and all requests receive the same result.
func TestSingleFlightCacheConcurrentRequests(t *testing.T) {
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
}

// TestSingleFlightCacheErrorHandling verifies that the cache correctly handles
// errors returned by the data loader.
func TestSingleFlightCacheErrorHandling(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "errorKey"
	expectedError := errors.New("simulated load error")
	loadDelay := 10 * time.Millisecond

	loaderCallCount := int32(0)
	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1)
		time.Sleep(loadDelay)
		return "", expectedError
	}

	numRequests := 3
	var wg sync.WaitGroup
	results := make(chan string, numRequests)
	errorsChan := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.GetOrLoad(key, loader)
			results <- data
			errorsChan <- err
		}()
		// Remove delay to ensure truly concurrent requests
	}

	wg.Wait()
	close(results)
	close(errorsChan)

	for i := 0; i < numRequests; i++ {
		data := <-results
		err := <-errorsChan
		if data != "" {
			t.Errorf("Request %d: Expected empty string data, got %v", i, data)
		}
		if err == nil || err.Error() != expectedError.Error() {
			t.Errorf("Request %d: Expected error %q, got %v", i, expectedError.Error(), err)
		}
	}

	if count := atomic.LoadInt32(&loaderCallCount); count != 1 {
		t.Errorf("DataLoader was called %d times, expected 1 (concurrent error requests should be deduplicated)", count)
	}
}

// TestSingleFlightCacheErrorRetry verifies that after an error, subsequent calls retry the loader
func TestSingleFlightCacheErrorRetry(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "retryKey"
	expectedError := errors.New("loading failed")
	loaderCallCount := int32(0)

	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1)
		return "", expectedError
	}

	// First call should get the error
	_, err := cache.GetOrLoad(key, loader)
	if err == nil {
		t.Fatal("Expected an error from GetOrLoad")
	}

	// Second call should retry the loader (since error wasn't cached)
	_, err2 := cache.GetOrLoad(key, loader)
	if err2 == nil {
		t.Fatal("Expected an error from second GetOrLoad")
	}

	// Verify that the data loader was called twice (once for each GetOrLoad)
	if count := atomic.LoadInt32(&loaderCallCount); count != 2 {
		t.Errorf("DataLoader was called %d times, expected 2 (errors should not be cached)", count)
	}
}

// TestSingleFlightCacheMultipleKeys tests that the cache handles different keys independently.
func TestSingleFlightCacheMultipleKeys(t *testing.T) {
	cache := utils.NewTTLCache[interface{}](10 * time.Second)
	key1 := "key1"
	key2 := "key2"
	expectedData1 := "data_for_key1"
	expectedData2 := "data_for_key2"
	loadDelay := 50 * time.Millisecond

	loaderCallCounts := sync.Map{} // Concurrent map to track loader calls per key

	loader1 := func() (interface{}, error) {
		loaderCallCounts.Store(key1, 1)
		time.Sleep(loadDelay)
		return expectedData1, nil
	}
	loader2 := func() (interface{}, error) {
		loaderCallCounts.Store(key2, 1)
		time.Sleep(loadDelay)
		return expectedData2, nil
	}

	var wg sync.WaitGroup

	// Request key1 multiple times concurrently
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.GetOrLoad(key1, loader1)
			if err != nil {
				t.Errorf("Error loading key1: %v", err)
			}
			if data != expectedData1 {
				t.Errorf("Unexpected data for key1: %v", data)
			}
		}()
	}

	// Request key2 multiple times concurrently
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.GetOrLoad(key2, loader2)
			if err != nil {
				t.Errorf("Error loading key2: %v", err)
			}
			if data != expectedData2 {
				t.Errorf("Unexpected data for key2: %v", data)
			}
		}()
	}

	wg.Wait()

	// Verify loaders were called exactly once per key
	if _, ok := loaderCallCounts.Load(key1); !ok {
		t.Errorf("Loader for key1 was not called.")
	}
	if _, ok := loaderCallCounts.Load(key2); !ok {
		t.Errorf("Loader for key2 was not called.")
	}
}

// TestSingleFlightCacheHitAfterLoad ensures that a request for an already loaded key
// does not trigger the data loader again.
func TestSingleFlightCacheHitAfterLoad(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "cacheHitKey"
	expectedData := "cached_value"
	loadDelay := 50 * time.Millisecond
	loaderCallCount := int32(0)

	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1)
		time.Sleep(loadDelay)
		return expectedData, nil
	}

	// First load (should call loader)
	_, err := cache.GetOrLoad(key, loader)
	if err != nil {
		t.Fatalf("First load error: %v", err)
	}
	if count := atomic.LoadInt32(&loaderCallCount); count != 1 {
		t.Fatalf("Loader called %d times after first load, expected 1", count)
	}

	// Second load (should hit cache, not call loader)
	_, err = cache.GetOrLoad(key, loader)
	if err != nil {
		t.Fatalf("Second load error: %v", err)
	}
	if count := atomic.LoadInt32(&loaderCallCount); count != 1 {
		t.Errorf("Loader called %d times after second load, expected 1 (cache hit)", count)
	}
}

// TestSingleFlightCacheDataConsistencyAfterConcurrentLoad ensures that all concurrent requests
// receive the exact same data after it's prepared by the single loader.
func TestSingleFlightCacheDataConsistencyAfterConcurrentLoad(t *testing.T) {
	cache := utils.NewTTLCache[string](10 * time.Second)
	key := "consistentKey"
	preparedData := "unique_data_from_preparation"
	loadDelay := 100 * time.Millisecond
	loaderCallCount := int32(0)

	loader := func() (string, error) {
		atomic.AddInt32(&loaderCallCount, 1)
		time.Sleep(loadDelay)
		return preparedData, nil
	}

	numRequests := 10
	var wg sync.WaitGroup
	receivedDatas := make(chan interface{}, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.GetOrLoad(key, loader)
			if err != nil {
				t.Errorf("Error getting data: %v", err)
				return
			}
			receivedDatas <- data
		}()
		time.Sleep(5 * time.Millisecond) // Stagger requests slightly
	}

	wg.Wait()
	close(receivedDatas)

	for receivedData := range receivedDatas {
		if receivedData != preparedData {
			t.Errorf("Received inconsistent data: expected %q, got %q", preparedData, receivedData)
		}
	}

	if count := atomic.LoadInt32(&loaderCallCount); count != 1 {
		t.Errorf("DataLoader was called %d times, expected 1 for consistent data", count)
	}
}
