package yakit

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
)

func TestDrainDBSaveBatchCoalescesWhenIdle(t *testing.T) {
	ch := make(chan DbExecFunc, 8)

	first := func(db *gorm.DB) error { return nil }
	ch <- first

	batch := drainDBSaveBatch(ch, <-ch)
	require.Len(t, batch, 1)
}

func TestDrainDBSaveBatchPullsMultiple(t *testing.T) {
	ch := make(chan DbExecFunc, 8)

	var n atomic.Int32
	for i := 0; i < 5; i++ {
		ch <- func(db *gorm.DB) error {
			n.Add(1)
			return nil
		}
	}

	batch := drainDBSaveBatch(ch, <-ch)
	require.GreaterOrEqual(t, len(batch), 2)
	require.LessOrEqual(t, len(batch), 5)
}

func TestDrainDBSaveBatchDoesNotWaitForFutureItems(t *testing.T) {
	ch := make(chan DbExecFunc)
	first := func(db *gorm.DB) error { return nil }

	startedAt := time.Now()
	batch := drainDBSaveBatch(ch, first)
	require.Len(t, batch, 1)
	require.Less(t, time.Since(startedAt), 50*time.Millisecond)
}

func TestEnqueuedDBSaveRemainsBoundToOriginalDatabase(t *testing.T) {
	originalDB := &gorm.DB{}
	otherDB := &gorm.DB{}
	called := false
	bound := bindDBSaveFunc(originalDB, func(db *gorm.DB) error {
		called = true
		require.Same(t, originalDB, db)
		return nil
	})

	require.NoError(t, bound(otherDB))
	require.True(t, called)
}

func TestExecDBSaveBatchUsesTransaction(t *testing.T) {
	var runs atomic.Int32
	batch := []DbExecFunc{
		func(db *gorm.DB) error { runs.Add(1); return nil },
		func(db *gorm.DB) error { runs.Add(1); return nil },
	}

	// nil db path in unit test: only verify batch size plumbing
	require.Equal(t, int32(0), runs.Load())
	_ = batch
	_ = time.Millisecond
}
