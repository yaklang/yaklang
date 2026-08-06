package yakgrpc

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryHTTPFlowsConcurrentWrite(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// A single SQLite connection avoids lock errors obscuring the shared gorm.DB
	// logger race this test is intended to cover.
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	server := &Server{projectDatabase: db}
	const (
		readers    = 3
		writers    = 2
		iterations = 100
	)

	start := make(chan struct{})
	errors := make(chan error, readers+writers)
	var wg sync.WaitGroup

	for reader := 0; reader < readers; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				_, queryErr := server.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
					Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "desc"},
				})
				if queryErr != nil {
					errors <- queryErr
					return
				}
			}
		}()
	}

	for writer := 0; writer < writers; writer++ {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				flow := &schema.HTTPFlow{
					Hash:       fmt.Sprintf("writer-%d-flow-%d", writer, i),
					Url:        fmt.Sprintf("http://example.test/%d/%d", writer, i),
					SourceType: schema.HTTPFlow_SourceType_MITM,
				}
				if writeErr := db.Create(flow).Error; writeErr != nil {
					errors <- writeErr
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errors)
	for testErr := range errors {
		require.NoError(t, testErr)
	}
}

func TestQueryHTTPFlowsConcurrentWriteWALPool(t *testing.T) {
	t.Setenv(consts.YakitSQLiteProjectMaxOpenConnsEnv, "2")
	db, err := consts.CreateProjectDatabase(filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.Equal(t, 2, db.DB().Stats().MaxOpenConnections)

	server := &Server{projectDatabase: db}
	const (
		readers    = 3
		writers    = 2
		iterations = 100
	)

	start := make(chan struct{})
	errors := make(chan error, readers+writers)
	var wg sync.WaitGroup

	for reader := 0; reader < readers; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				_, queryErr := server.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
					SkipTotal:  true,
					Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "desc"},
				})
				if queryErr != nil {
					errors <- queryErr
					return
				}
			}
		}()
	}

	for writer := 0; writer < writers; writer++ {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				flow := &schema.HTTPFlow{
					Url:        fmt.Sprintf("http://example.test/wal/%d/%d", writer, i),
					SourceType: schema.HTTPFlow_SourceType_MITM,
				}
				if writeErr := db.Create(flow).Error; writeErr != nil {
					errors <- writeErr
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errors)
	for testErr := range errors {
		require.NoError(t, testErr)
	}

	var count int
	require.NoError(t, db.Model(&schema.HTTPFlow{}).Count(&count).Error)
	require.Equal(t, writers*iterations, count)
}

func TestQueryHTTPFlowsConcurrentWriteDedicatedReadPool(t *testing.T) {
	t.Setenv(consts.YakitSQLiteProjectMaxOpenConnsEnv, "1")
	t.Setenv(consts.YakitSQLiteProjectReadPoolConnsEnv, "1")
	path := filepath.Join(t.TempDir(), "project.db")
	db, err := consts.CreateProjectDatabase(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	readDB, err := consts.CreateProjectDatabaseReadOnly(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readDB.Close() })
	require.Equal(t, 1, db.DB().Stats().MaxOpenConnections)
	require.Equal(t, 1, readDB.DB().Stats().MaxOpenConnections)

	server := &Server{projectDatabase: db, projectReadDatabase: readDB}
	const (
		readers    = 3
		iterations = 200
	)

	start := make(chan struct{})
	errors := make(chan error, readers+1)
	var wg sync.WaitGroup
	for reader := 0; reader < readers; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				_, queryErr := server.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
					SkipTotal:  true,
					Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "desc"},
				})
				if queryErr != nil {
					errors <- queryErr
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			flow := &schema.HTTPFlow{
				Url:        fmt.Sprintf("http://example.test/dedicated-reader/%d", i),
				SourceType: schema.HTTPFlow_SourceType_MITM,
			}
			if writeErr := db.Create(flow).Error; writeErr != nil {
				errors <- writeErr
				return
			}
		}
	}()

	close(start)
	wg.Wait()
	close(errors)
	for testErr := range errors {
		require.NoError(t, testErr)
	}
	var count int
	require.NoError(t, db.Model(&schema.HTTPFlow{}).Count(&count).Error)
	require.Equal(t, iterations, count)
}
