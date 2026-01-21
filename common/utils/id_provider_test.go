package utils_test

import (
	"sync"
	"testing"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAtomicInt64IDProvider_Concurrent(t *testing.T) {
	p := utils.NewAtomicInt64IDProvider(1)

	const (
		goroutines = 32
		perWorker  = 256
		total      = goroutines * perWorker
	)

	out := make(chan int64, total)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				out <- p.NewID()
			}
		}()
	}
	wg.Wait()
	close(out)

	seen := make(map[int64]struct{}, total)
	var min, max int64
	first := true
	for id := range out {
		_, exists := seen[id]
		require.False(t, exists, "duplicate id: %d", id)
		seen[id] = struct{}{}
		if first {
			min, max = id, id
			first = false
			continue
		}
		if id < min {
			min = id
		}
		if id > max {
			max = id
		}
	}

	require.Len(t, seen, total)
	require.Equal(t, int64(1), min)
	require.Equal(t, int64(total), max)
}

func TestKSUIDProviders(t *testing.T) {
	t.Run("ksuid", func(t *testing.T) {
		p := utils.NewKSUIDProvider()
		require.Equal(t, ksuid.Nil, p.CurrentID())

		id := p.NewID()
		require.NotEmpty(t, id.String())
		require.NotEqual(t, ksuid.Nil, id)
		require.Equal(t, id, p.CurrentID())
	})

	t.Run("string", func(t *testing.T) {
		p := utils.NewKSUIDStringProvider()
		require.Empty(t, p.CurrentID())

		a := p.NewID()
		require.Equal(t, a, p.CurrentID())

		b := p.NewID()
		require.NotEmpty(t, a)
		require.NotEmpty(t, b)
		require.NotEqual(t, a, b)
		require.Equal(t, b, p.CurrentID())

		_, err := ksuid.Parse(a)
		require.NoError(t, err)
	})
}
