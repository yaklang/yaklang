package rag

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMUSTPASS_CheckConfigEmbeddingAvailable_ConcurrentSingleflightPerModel(t *testing.T) {
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

func TestMUSTPASS_CheckConfigEmbeddingAvailable_NegativeCacheTTL(t *testing.T) {
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

	// Exercise the local negative cache independently of remote fallback availability.
	checker := newEmbeddingAvailableChecker("missing-model")
	t.Cleanup(checker.close)

	if checker.check() {
		t.Fatalf("expected unavailable")
	}
	if checker.check() {
		t.Fatalf("expected unavailable")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected getModelPath called once within TTL, got %d", got)
	}

	time.Sleep(embeddingAvailabilityNegativeCacheTTL + 20*time.Millisecond)
	if checker.check() {
		t.Fatalf("expected unavailable")
	}
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Fatalf("expected getModelPath called again after TTL, got %d", got)
	}
}

// Benchmark the warm availability path without network or model files.
func BenchmarkCheckConfigEmbeddingAvailableCached(b *testing.B) {
	clearEmbeddingAvailableCache()
	oldGetModelPath := getModelPath
	getModelPath = func(string) (string, error) { return "/tmp/fake-model.bin", nil }
	b.Cleanup(func() {
		clearEmbeddingAvailableCache()
		getModelPath = oldGetModelPath
	})
	option := WithModelName("cached-benchmark-model")
	if !CheckConfigEmbeddingAvailable(option) {
		b.Fatal("expected available")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !CheckConfigEmbeddingAvailable(option) {
			b.Fatal("cached model became unavailable")
		}
	}
}
