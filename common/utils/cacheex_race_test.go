package utils

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestCacheExConcurrentStress exercises the cache concurrently from many
// goroutines together with the background expiration/cleanup goroutines.
// It is meant to be run with the race detector to catch unsynchronized
// access to the cache's internal mutable state.
func TestCacheExConcurrentStress(t *testing.T) {
	cache := NewCacheExWithKey[string, int](WithCacheTTL(5 * time.Millisecond))
	defer cache.Close()

	const workers = 16
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// writers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("k-%d-%d", id, i%32)
				cache.Set(key, i)
				cache.SetWithTTL(key, i, 3*time.Millisecond)
				i++
			}
		}(w)
	}

	// readers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("k-%d-%d", id, i%32)
				_, _ = cache.Get(key)
				_ = cache.Count()
				_ = cache.GetAll()
				i++
			}
		}(w)
	}

	// single-flight loaders
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("sf-%d", i%8)
				_, _ = cache.GetOrLoad(key, func() (int, error) {
					return i, nil
				})
				i++
			}
		}(w)
	}

	// configuration mutators racing with the background goroutines
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			cache.SetExpirationCallback(func(key string, value int, reason EvictionReason) {
				_ = key
				_ = value
			})
			cache.SetNewItemCallback(func(key string, value int) {})
			cache.SetTTL(time.Duration(2+(i%5)) * time.Millisecond)
			cache.SkipTtlExtensionOnHit(i%2 == 0)
			i++
			time.Sleep(time.Microsecond)
		}
	}()

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()
}

// TestCacheWithKeyConcurrentStress exercises the CacheWithKey wrapper (cache.go)
// whose expiration callback is invoked from the background eviction goroutine
// while SetExpirationCallback is called concurrently.
func TestCacheWithKeyConcurrentStress(t *testing.T) {
	cache := NewTTLCacheWithKey[string, int](5 * time.Millisecond)
	defer cache.Close()

	const workers = 12
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				key := fmt.Sprintf("k-%d-%d", id, i%16)
				cache.Set(key, i)
				_, _ = cache.Get(key)
				i++
			}
		}(w)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			cache.SetExpirationCallback(func(key string, value int) {
				_ = key
				_ = value
			})
			time.Sleep(time.Microsecond)
		}
	}()

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()
}
