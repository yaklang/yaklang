package rag

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckConfigEmbeddingAvailable_ConcurrentSingleflightPerModel(t *testing.T) {
	t.Cleanup(clearEmbeddingAvailableCache)
	clearEmbeddingAvailableCache()

	oldGetModelPath := getModelPath
	oldTTL := embeddingAvailabilityNegativeCacheTTL
	t.Cleanup(func() {
		getModelPath = oldGetModelPath
		embeddingAvailabilityNegativeCacheTTL = oldTTL
	})

	embeddingAvailabilityNegativeCacheTTL = 50 * time.Millisecond

	var calls int64
	getModelPath = func(modelName string) (string, error) {
		atomic.AddInt64(&calls, 1)
		time.Sleep(30 * time.Millisecond)
		return "/tmp/fake-model.bin", nil
	}

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if !CheckConfigEmbeddingAvailable(WithModelName("test-model")) {
				errCh <- fmt.Errorf("expected available")
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected getModelPath to be called once, got %d", got)
	}
}

func TestCheckConfigEmbeddingAvailable_NegativeCacheTTL(t *testing.T) {
	t.Cleanup(clearEmbeddingAvailableCache)
	clearEmbeddingAvailableCache()

	oldGetModelPath := getModelPath
	oldTTL := embeddingAvailabilityNegativeCacheTTL
	t.Cleanup(func() {
		getModelPath = oldGetModelPath
		embeddingAvailabilityNegativeCacheTTL = oldTTL
	})

	embeddingAvailabilityNegativeCacheTTL = 50 * time.Millisecond

	var calls int64
	getModelPath = func(modelName string) (string, error) {
		atomic.AddInt64(&calls, 1)
		return "", fmt.Errorf("not found")
	}

	if CheckConfigEmbeddingAvailable(WithModelName("missing-model")) {
		t.Fatalf("expected unavailable")
	}
	if CheckConfigEmbeddingAvailable(WithModelName("missing-model")) {
		t.Fatalf("expected unavailable")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected getModelPath called once within TTL, got %d", got)
	}

	time.Sleep(embeddingAvailabilityNegativeCacheTTL + 20*time.Millisecond)
	if CheckConfigEmbeddingAvailable(WithModelName("missing-model")) {
		t.Fatalf("expected unavailable")
	}
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Fatalf("expected getModelPath called again after TTL, got %d", got)
	}
}
