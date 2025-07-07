package utils_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAsyncFactory(t *testing.T) {
	t.Run("basic get", func(t *testing.T) {
		var counter int32
		factory := func() (int, error) {
			atomic.AddInt32(&counter, 1)
			return int(atomic.LoadInt32(&counter)), nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		af := utils.NewAsyncFactory(ctx, factory)
		defer af.Close()

		item, err := af.Get()
		assert.NoError(t, err)
		assert.Equal(t, 1, item)

		item, err = af.Get()
		assert.NoError(t, err)
		assert.Equal(t, 2, item)
	})

	t.Run("concurrent get", func(t *testing.T) {
		var counter int32
		factory := func() (int, error) {
			time.Sleep(time.Millisecond)
			return int(atomic.AddInt32(&counter, 1)), nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		af := utils.NewAsyncFactory(ctx, factory)
		defer af.Close()

		var wg sync.WaitGroup
		numGets := 100
		wg.Add(numGets)

		results := make(chan int, numGets)
		for i := 0; i < numGets; i++ {
			go func() {
				defer wg.Done()
				item, err := af.Get()
				if err == nil {
					results <- item
				}
			}()
		}

		wg.Wait()
		close(results)

		received := make(map[int]bool)
		for item := range results {
			assert.False(t, received[item], "received duplicate item %d", item)
			received[item] = true
		}

		assert.Equal(t, numGets, len(received))
	})

	t.Run("close", func(t *testing.T) {
		factory := func() (int, error) {
			return 1, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		af := utils.NewAsyncFactory(ctx, factory)
		af.Close()

		_, err := af.Get()
		assert.Error(t, err)
	})

	t.Run("context cancel", func(t *testing.T) {
		factory := func() (int, error) {
			time.Sleep(10 * time.Millisecond)
			return 1, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		af := utils.NewAsyncFactory(ctx, factory)
		defer af.Close()

		// Let the factory produce some items
		time.Sleep(50 * time.Millisecond)

		cancel()

		// After context is canceled, Get should eventually return an error.
		// We will try to get a few times, to exhaust any buffered items.
		var err error
		for i := 0; i < 300; i++ {
			_, err = af.Get()
			if err != nil {
				break
			}
		}
		assert.Error(t, err)
	})
}
