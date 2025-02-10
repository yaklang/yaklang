package bizhelper

import (
	"fmt"
	"os"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type KV struct {
	gorm.Model
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
