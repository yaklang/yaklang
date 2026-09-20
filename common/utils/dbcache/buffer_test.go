package dbcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSmallInitialSaveBufferPreservesAllItems(t *testing.T) {
	for _, initial := range []int{0, 1, 64} {
		var saved []int
		s := NewSave(func(items []int) error {
			saved = append(saved, items...)
			return nil
		}, WithInitialBufferSize(initial), WithSaveSize(1000), WithSaveTimeout(time.Hour))
		for i := 0; i < 10000; i++ {
			s.Save(i)
		}
		require.NoError(t, s.Close())
		require.Len(t, saved, 10000)
		for i, value := range saved {
			require.Equal(t, i, value)
		}
	}
}

func TestInitialBufferDoesNotChangeBatches(t *testing.T) {
	defaults := NewConfig(WithSaveSize(2000))
	small := NewConfig(WithInitialBufferSize(64), WithSaveSize(2000))
	require.Equal(t, defaults.saveSize, small.saveSize)
	require.Equal(t, defaults.fetchSize, small.fetchSize)
	require.Equal(t, 64, small.bufferSize)
	require.Equal(t, 4*max(defaults.fetchSize, defaults.saveSize), defaults.bufferSize)
}

func BenchmarkSmallResultSaveQueue(b *testing.B) {
	for _, initial := range []int{0, 64} {
		name := "default"
		if initial != 0 {
			name = "small"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s := NewSave(func([]*int) error { return nil }, WithInitialBufferSize(initial), WithSaveSize(200))
				v := 1
				s.Save(&v)
				if err := s.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
