package databasex_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/databasex"
)

// FetchTestItem is a simple implementation of the Item interface for testing
type FetchTestItem struct {
	ID   int
	Name string
}

func TestNewFetch(t *testing.T) {
	mockItems := []FetchTestItem{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
		{ID: 3, Name: "Item 3"},
	}

	// Test with default options
	t.Run("DefaultOptions", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return mockItems
		}

		fetch := databasex.NewFetch(fetchFromDB)
		assert.NotNil(t, fetch)

		// Close the fetch to clean up resources
		fetch.Close()
	})

	// Test with custom buffer size
	t.Run("CustomBufferSize", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return mockItems
		}

		fetch := databasex.NewFetch(fetchFromDB,
			databasex.WithBufferSize(10),
		)
		assert.NotNil(t, fetch)

		// Close the fetch to clean up resources
		fetch.Close()
	})

	// Test with custom context
	t.Run("CustomContext", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return mockItems
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		fetch := databasex.NewFetch(fetchFromDB,
			databasex.WithContext(ctx),
		)
		assert.NotNil(t, fetch)

		// Close the fetch to clean up resources
		fetch.Close()
	})
}

func TestFetchOperation(t *testing.T) {
	mockItems := []FetchTestItem{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
		{ID: 3, Name: "Item 3"},
	}

	t.Run("FetchItems", func(t *testing.T) {
		var fetchCount int
		fetchFromDB := func() []FetchTestItem {
			fetchCount++
			return mockItems
		}

		fetch := databasex.NewFetch(fetchFromDB)
		assert.NotNil(t, fetch)

		// Fetch items
		for i := 0; i < 3; i++ {
			item, err := fetch.Fetch()
			assert.NoError(t, err)
			assert.NotNil(t, item)
			assert.Contains(t, mockItems, item)
		}

		// Close the fetch to clean up resources
		fetch.Close()
	})

	t.Run("EmptyFetch", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return []FetchTestItem{} // Return empty slice
		}

		fetch := databasex.NewFetch(fetchFromDB)
		assert.NotNil(t, fetch)

		// Wait a bit to ensure the buffer has had time to try filling
		time.Sleep(100 * time.Millisecond)

		// Close the fetch since we don't expect to get any items
		fetch.Close()
	})
}

func TestCloseWithDelete(t *testing.T) {
	mockItems := []FetchTestItem{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
		{ID: 3, Name: "Item 3"},
	}

	t.Run("DeleteOnClose", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return mockItems
		}

		var deletedItems []FetchTestItem
		deleteFunc := func(items []FetchTestItem) {
			deletedItems = append(deletedItems, items...)
		}

		fetch := databasex.NewFetch(fetchFromDB)
		assert.NotNil(t, fetch)

		// Wait a bit to ensure the buffer is filled
		time.Sleep(100 * time.Millisecond)

		// Close with delete function
		fetch.Close(deleteFunc)

		// Check if items were deleted correctly
		assert.NotEmpty(t, deletedItems)
	})
}

func TestConcurrency(t *testing.T) {
	// This test ensures that the fetch operation works correctly
	// when multiple goroutines are trying to fetch items
	mockItems := []FetchTestItem{
		{ID: 1, Name: "Item 1"},
		{ID: 2, Name: "Item 2"},
		{ID: 3, Name: "Item 3"},
	}

	t.Run("ConcurrentFetch", func(t *testing.T) {
		fetchFromDB := func() []FetchTestItem {
			return mockItems
		}

		fetch := databasex.NewFetch(fetchFromDB, databasex.WithBufferSize(100))
		assert.NotNil(t, fetch)

		var wg sync.WaitGroup
		itemCount := 10
		workers := 5

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < itemCount; i++ {
					item, err := fetch.Fetch()
					assert.NoError(t, err)
					assert.NotNil(t, item)
					assert.Contains(t, mockItems, item)
				}
			}()
		}

		wg.Wait()
		fetch.Close()
	})
}
