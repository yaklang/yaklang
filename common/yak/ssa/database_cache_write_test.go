package ssa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSQLiteCacheWritesMayOverlap(t *testing.T) {
	for _, dialect := range []string{"sqlite3", "sqlite3_extended"} {
		t.Run(dialect, func(t *testing.T) {
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			finished := make(chan error, 2)
			enter := func() error {
				entered <- struct{}{}
				<-release
				return nil
			}
			go func() { finished <- withCacheWriteLock(dialect, enter) }()
			go func() { finished <- withCacheWriteLock(dialect, enter) }()

			got := 0
			timeout := time.After(2 * time.Second)
			for got < 2 {
				select {
				case <-entered:
					got++
				case <-timeout:
					close(release)
					t.Fatalf("SQLite cache writers did not overlap: entered=%d", got)
				}
			}
			close(release)
			for i := 0; i < 2; i++ {
				err := <-finished
				require.NoError(t, err)
			}
		})
	}
}
