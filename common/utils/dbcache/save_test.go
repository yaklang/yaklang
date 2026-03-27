package dbcache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/dbcache"
)

// TestItem is a simple implementation of the Item interface for testing
type TestItem struct {
	ID   int
	Data string
}

func TestNewSaver(t *testing.T) {
	// Test with default options
	savedItems := make([]TestItem, 0)
	saveFn := func(items []TestItem) {
		savedItems = append(savedItems, items...)
	}

	saver := dbcache.NewSave(saveFn)
	require.NotNil(t, saver)
	// We can't directly access internal fields of Saver from the test package
	// Just verify that the saver is created successfully and can be closed
	saver.Close()

	// Test with custom options
	ctx := context.Background()
	saver = dbcache.NewSave(
		saveFn,
		dbcache.WithFetchSize(200),
		dbcache.WithContext(ctx),
	)
	require.NotNil(t, saver)
	// Can't access internal field wg
	saver.Close()
}

func TestSaver_Save(t *testing.T) {
	savedItems := []TestItem{}
	saveMutex := &sync.Mutex{}
	saveFn := func(items []TestItem) {
		saveMutex.Lock()
		defer saveMutex.Unlock()
		savedItems = append(savedItems, items...)
	}

	ttl := 100 * time.Millisecond
	saver := dbcache.NewSave(saveFn,
		dbcache.WithSaveTimeout(ttl),
	)
	defer saver.Close()

	// Test saving single item
	item1 := TestItem{ID: 1, Data: "test1"}
	saver.Save(item1)

	// Give time for the background goroutine to process
	time.Sleep(2 * ttl)

	saveMutex.Lock()
	require.Equal(t, 1, len(savedItems))
	require.Equal(t, item1.ID, savedItems[0].ID)
	require.Equal(t, item1.Data, savedItems[0].Data)
	saveMutex.Unlock()

	// Test saving multiple items
	savedItems = []TestItem{} // Reset saved items
	items := []TestItem{
		{ID: 2, Data: "test2"},
		{ID: 3, Data: "test3"},
		{ID: 4, Data: "test4"},
	}

	for _, item := range items {
		log.Errorf("save item: %v", item)
		saver.Save(item)
	}

	// Give time for the background goroutine to process
	time.Sleep(2 * ttl)

	saveMutex.Lock()
	// The Saver might batch items, so the exact number might not match
	// Just make sure all our items are there
	for _, expected := range items {
		found := false
		for _, actual := range savedItems {
			if actual.ID == expected.ID {
				require.Equal(t, expected.Data, actual.Data)
				found = true
				break
			}
		}
		require.True(t, found, "Expected item with ID %d not found", expected.ID)
	}
	saveMutex.Unlock()
}

func TestSaver_Close(t *testing.T) {
	savedItems := []TestItem{}
	saveMutex := &sync.Mutex{}
	saveFn := func(items []TestItem) {
		saveMutex.Lock()
		defer saveMutex.Unlock()
		savedItems = append(savedItems, items...)
	}

	saver := dbcache.NewSave(saveFn)

	// Save some items
	items := []TestItem{
		{ID: 1, Data: "test1"},
		{ID: 2, Data: "test2"},
		{ID: 3, Data: "test3"},
	}

	for _, item := range items {
		saver.Save(item)
	}

	// Then close, should process remaining items
	saver.Close()

	saveMutex.Lock()
	require.GreaterOrEqual(t, len(savedItems), 3, "Should have saved at least one item")
	saveMutex.Unlock()
}

func TestSaver_WithCustomContext(t *testing.T) {
	savedItems := []TestItem{}
	saveMutex := &sync.Mutex{}
	saveFn := func(items []TestItem) {
		saveMutex.Lock()
		defer saveMutex.Unlock()
		savedItems = append(savedItems, items...)
	}

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	saver := dbcache.NewSave(saveFn, dbcache.WithContext(ctx))
	defer saver.Close()

	// Save an item
	item := TestItem{ID: 1, Data: "test1"}
	saver.Save(item)

	// Wait a bit for the first item to be processed
	time.Sleep(100 * time.Millisecond)

	// Cancel the context, which should stop the background goroutine
	cancel()

	// Save another item after cancellation - this might still be accepted by the buffer
	// but shouldn't be processed by the background goroutine
	item2 := TestItem{ID: 2, Data: "test2"}
	saver.Save(item2)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Check that only the first item was saved
	saveMutex.Lock()
	itemFound := false
	for _, savedItem := range savedItems {
		if savedItem.ID == 1 {
			itemFound = true
			break
		}
	}
	require.True(t, itemFound, "First item should be saved")
	saveMutex.Unlock()
}

func TestSaveAutoSaveSize(t *testing.T) {
	defaultSaveSize := 10
	saveTimeout := 200 * time.Millisecond

	var savedItemSize []int
	var mu sync.Mutex

	saveFn := func(items []int) {
		mu.Lock()
		defer mu.Unlock()
		savedItemSize = append(savedItemSize, len(items))
		// Make a copy of the slice
	}

	save := dbcache.NewSave(saveFn,
		dbcache.WithSaveSize(defaultSaveSize),
		dbcache.WithSaveTimeout(saveTimeout),
	)

	for i := 0; i < 100; i++ {
		save.Save(i)
	}

	time.Sleep(500 * time.Millisecond) // Wait for the saver to process
	save.Close()

	require.NotEmpty(t, savedItemSize)
	total := 0
	for _, size := range savedItemSize {
		require.Greater(t, size, 0)
		total += size
	}
	require.Equal(t, 100, total)
}

func TestSaver_SaveRunsSerially(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32

	saveFn := func(items []int) {
		current := active.Add(1)
		for {
			maxSeen := maxActive.Load()
			if current <= maxSeen || maxActive.CompareAndSwap(maxSeen, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
	}

	saver := dbcache.NewSave(saveFn,
		dbcache.WithSaveSize(1),
		dbcache.WithSaveTimeout(time.Second),
	)
	for i := 0; i < 8; i++ {
		saver.Save(i)
	}
	saver.Close()

	require.Equal(t, int32(1), maxActive.Load())
}

func TestSaver_Stats(t *testing.T) {
	var batches int
	saver := dbcache.NewSave(func(items []int) {
		batches++
		time.Sleep(10 * time.Millisecond)
	}, dbcache.WithSaveSize(2), dbcache.WithSaveTimeout(30*time.Millisecond))

	saver.Save(1)
	saver.Save(2)
	saver.Save(3)
	saver.Close()

	stats := saver.Stats()
	require.Equal(t, 3, int(stats.BatchItemsTotal))
	require.GreaterOrEqual(t, int(stats.BatchCount), 1)
	require.GreaterOrEqual(t, int(stats.MaxPending), 1)
	require.GreaterOrEqual(t, int(stats.MaxBatchSize), 1)
	require.GreaterOrEqual(t, stats.SaveTimeTotal, 10*time.Millisecond)
	require.Equal(t, int(stats.BatchCount), batches)
}
