package bizhelper

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type KV struct {
	gorm.Model
	Key   string
	Value string
}

type KVNoID struct {
	Key   string
	Value string
}

func TestFastPaginator(t *testing.T) {
	// Create a temporary file for the database
	tempFile, err := os.CreateTemp("", "testdb_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() {
		os.Remove(tempFile.Name())
	})

	// Open the database
	db, err := gorm.Open("sqlite3", tempFile.Name())
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// Migrate the schema

	if err = db.AutoMigrate(&KV{}).Error; err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert test data
	for i := 0; i < 100; i++ {
		db.Create(&KV{Key: fmt.Sprintf("key%d", i), Value: fmt.Sprintf("value%d", i)})
	}

	// size = 100
	{
		p := NewFastPaginator(db.Model(&KV{}), 100)
		// first
		var items []KV
		err, ok := p.Next(&items)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, items, 100)

		// second
		items = make([]KV, 0)
		err, ok = p.Next(&items)
		require.NoError(t, err)
		require.False(t, ok)
		require.Len(t, items, 0)
	}

	// size = 99
	{
		p := NewFastPaginator(db.Model(&KV{}), 99)
		// first
		var items []KV
		err, ok := p.Next(&items)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, items, 99)

		// second
		items = make([]KV, 0)
		err, ok = p.Next(&items)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, items, 1)
		item := items[0]
		require.Equal(t, "key99", item.Key)
		require.Equal(t, "value99", item.Value)
	}

	// size = 50
	{
		p := NewFastPaginator(db.Model(&KV{}), 50)
		// first
		var items []KV
		err, ok := p.Next(&items)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, items, 50)

		// second
		items = make([]KV, 0)
		err, ok = p.Next(&items)
		require.NoError(t, err)
		require.True(t, ok)
		require.Len(t, items, 50)
		item := items[0]
		require.Equal(t, "key50", item.Key)
		require.Equal(t, "value50", item.Value)

		// third
		items = make([]KV, 0)
		err, ok = p.Next(&items)
		require.NoError(t, err)
		require.False(t, ok)
	}
}

func TestFastPaginatorVSCommonPaginator(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	if err = db.AutoMigrate(&KV{}).Error; err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert massive data
	total := 10000
	batchSize := 100
	// Use transaction for faster insertion
	tx := db.Begin()
	for i := 0; i < total; i++ {
		tx.Create(&KV{Key: fmt.Sprintf("key%d", i), Value: fmt.Sprintf("value%d", i)})
		if (i+1)%batchSize == 0 {
			tx.Commit()
			tx = db.Begin()
		}
	}
	tx.Commit()

	pageSize := 101

	// Scenario 1: Traverse all pages (creating paginator once vs creating repeatedly)
	t.Run("TraverseAll_CommonPaginator", func(t *testing.T) {
		start := time.Now()
		page := 1
		for {
			var items []KV
			param := &Param{
				DB:    db.Model(&KV{}),
				Page:  page,
				Limit: pageSize,
			}
			paginator, _ := NewPagination(param, &items)
			if len(items) == 0 {
				break
			}
			if page >= paginator.TotalPage {
				break
			}
			page++
		}
		log.Infof("Traverse All Pages - Common Paginator took: %v", time.Since(start))
	})

	t.Run("TraverseAll_FastPaginator", func(t *testing.T) {
		start := time.Now()
		p := NewFastPaginator(db.Model(&KV{}), pageSize)
		for {
			var items []KV
			err, ok := p.Next(&items)
			require.NoError(t, err)
			if !ok {
				break
			}
		}
		log.Infof("Traverse All Pages - Fast Paginator took: %v", time.Since(start))
	})

	t.Run("TraverseAll_CommonPaginator_Next", func(t *testing.T) {
		start := time.Now()
		var items []KV
		param := &Param{
			DB:    db.Model(&KV{}),
			Page:  1,
			Limit: pageSize,
		}
		p, _ := NewPagination(param, &items)
		for {
			var items []KV
			err, ok := p.Next(&items)
			require.NoError(t, err)
			if !ok {
				break
			}
		}
		log.Infof("Traverse All Pages - Common Paginator (Next) took: %v", time.Since(start))
	})

	t.Run("TraverseAll_CommonPaginator_Next_QueryCountOnce", func(t *testing.T) {
		start := time.Now()
		var items []KV
		param := &Param{
			DB:             db.Model(&KV{}),
			Page:           1,
			Limit:          pageSize,
			QueryCountOnce: true,
		}
		p, _ := NewPagination(param, &items)
		for {
			var items []KV
			err, ok := p.Next(&items)
			require.NoError(t, err)
			if !ok {
				break
			}
		}
		log.Infof("Traverse All Pages - Common Paginator (Next+QueryCountOnce) took: %v", time.Since(start))
	})

	t.Run("TraverseAll_CommonPaginator_Next_QueryCountOnce_DisableTransaction", func(t *testing.T) {
		start := time.Now()
		var items []KV
		param := &Param{
			DB:                 db.Model(&KV{}),
			Page:               1,
			Limit:              pageSize,
			QueryCountOnce:     true,
			DisableTransaction: true,
		}
		p, _ := NewPagination(param, &items)
		for {
			var items []KV
			err, ok := p.Next(&items)
			require.NoError(t, err)
			if !ok {
				break
			}
		}
		log.Infof("Traverse All Pages - Common Paginator (Next+QueryCountOnce+DisableTransaction) took: %v", time.Since(start))
	})

	t.Run("TraverseAll_SeekPagination", func(t *testing.T) {
		start := time.Now()
		var lastID uint = 0
		for {
			var items []KV
			// Seek Pagination: WHERE id > lastID ORDER BY id ASC LIMIT pageSize
			if err := db.Model(&KV{}).Where("id > ?", lastID).Order("id asc").Limit(pageSize).Find(&items).Error; err != nil {
				t.Fatal(err)
			}
			if len(items) == 0 {
				break
			}
			lastID = items[len(items)-1].ID
		}
		log.Infof("Traverse All Pages - Seek Pagination (id > last_id) took: %v", time.Since(start))
	})

	t.Run("TraverseAll_IDRangePagination", func(t *testing.T) {
		start := time.Now()
		// Assuming we know the IDs are continuous 1..10000
		startID := 1
		for {
			var items []KV
			endID := startID + pageSize - 1
			// Range Pagination: WHERE id >= startID AND id <= endID
			if err := db.Model(&KV{}).Where("id >= ? AND id <= ?", startID, endID).Find(&items).Error; err != nil {
				t.Fatal(err)
			}
			if len(items) == 0 {
				break
			}
			startID += pageSize
			if startID > total { // Need to know total or break on empty items
				break
			}
		}
		log.Infof("Traverse All Pages - ID Range Pagination (id >= start && id <= end) took: %v", time.Since(start))
	})

	// Scenario 2: Repeatedly query the first page (simulating high concurrency on list view)
	// This tests the overhead of initialization for each request
	loopCount := 100
	t.Run("RepeatedFirstPage_CommonPaginator", func(t *testing.T) {
		start := time.Now()
		for i := 0; i < loopCount; i++ {
			var items []KV
			param := &Param{
				DB:    db.Model(&KV{}),
				Page:  1,
				Limit: pageSize,
			}
			NewPagination(param, &items)
		}
		log.Infof("Repeated First Page (%d times) - Common Paginator took: %v", loopCount, time.Since(start))
	})

	t.Run("RepeatedFirstPage_FastPaginator", func(t *testing.T) {
		start := time.Now()
		for i := 0; i < loopCount; i++ {
			p := NewFastPaginator(db.Model(&KV{}), pageSize)
			var items []KV
			p.Next(&items)
		}
		log.Infof("Repeated First Page (%d times) - Fast Paginator took: %v", loopCount, time.Since(start))
	})

	// Traverse All Pages - Common Paginator took: 220.961541ms
	// Traverse All Pages - Fast Paginator took: 143.7625ms
	// Repeated First Page (100 times) - Common Paginator took: 216.6025ms
	// Repeated First Page (100 times) - Fast Paginator took: 1.293810375s

	// 运行结果显示：
	// 遍历全量数据：两者性能相近（在 10k 数据量下），具体取决于数据库状态。
	// 只查第一页：CommonPaginator 显著快于 FastPaginator，因为 FastPaginator 在初始化时总是会查询并加载所有 IDs，这在只查第一页时是不必要的开销。
}
