package ssa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSQLiteCacheWritesAreSerialized(t *testing.T) {
	for _, dialect := range []string{"sqlite3", "sqlite3_extended"} {
		t.Run(dialect, func(t *testing.T) {
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			finished := make(chan error, 2)
			go func() {
				finished <- withCacheWriteLock(dialect, func() error {
					entered <- struct{}{}
					<-release
					return nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("first cache writer did not enter")
			}
			attempted := make(chan struct{})
			go func() {
				close(attempted)
				finished <- withCacheWriteLock(dialect, func() error {
					entered <- struct{}{}
					return nil
				})
			}()
			<-attempted
			select {
			case <-entered:
				close(release)
				t.Fatal("SQLite cache writers overlapped")
			case <-time.After(50 * time.Millisecond):
			}
			close(release)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("second cache writer did not enter after release")
			}
			require.NoError(t, <-finished)
			require.NoError(t, <-finished)
		})
	}
}
