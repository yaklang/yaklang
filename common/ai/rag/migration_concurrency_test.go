package rag

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestConcurrentRAGMigration(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "fresh.db"))
	require.NoError(t, err)
	defer db.Close()
	db.LogMode(false)
	db.DB().SetMaxOpenConns(1)
	const workers = 8
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errors <- autoMigrateRAGSystem(db)
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
