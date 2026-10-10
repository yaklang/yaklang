package pcapdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestPCAPDBMultipleReadersSingleWriter(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	other, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer other.Close()
	fresh, err := other.Open(context.Background(), db.ID)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lock, err := acquireFileLock(ctx, datasetLockPath(meta), true)
	require.NoError(t, err)
	defer releaseFileLock(lock)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	require.Equal(t, 1, writer.DB().Stats().MaxOpenConnections)
	var journal string
	var synchronous, fullfsync, foreignKeys int
	require.NoError(t, writer.Raw("PRAGMA journal_mode").Row().Scan(&journal))
	require.NoError(t, writer.Raw("PRAGMA synchronous").Row().Scan(&synchronous))
	require.NoError(t, writer.Raw("PRAGMA fullfsync").Row().Scan(&fullfsync))
	require.NoError(t, writer.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys))
	require.Equal(t, "wal", journal)
	require.Equal(t, 2, synchronous)
	require.Equal(t, 1, fullfsync)
	require.Equal(t, 1, foreignKeys)

	// Pin all four physical read connections and establish their snapshots
	// before the writer starts. A sequential series of reads would not prove
	// that several readers can remain active during a commit.
	readers := make([]*gorm.DB, 4)
	for i := range readers {
		readers[i] = db.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		require.NoError(t, readers[i].Error)
		defer readers[i].Rollback()
		var queryOnly int
		require.NoError(t, readers[i].Raw("PRAGMA query_only").Row().Scan(&queryOnly))
		require.Equal(t, 1, queryOnly)
		var packet Packet
		require.NoError(t, readers[i].First(&packet, 1).Error)
		require.Empty(t, packet.DecodeError)
	}
	require.Equal(t, 4, db.db.DB().Stats().InUse)
	require.Equal(t, 4, db.db.DB().Stats().MaxOpenConnections)

	tx := writer.BeginTx(ctx, nil)
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, tx.Model(&Packet{}).Where("id = ?", 1).Update("decode_error", "committed by writer").Error)
	require.NoError(t, writeManifest(ctx, tx, meta))
	rows, err := fresh.QueryPackets(QueryContext(ctx))
	require.NoError(t, err)
	require.Empty(t, rows[0].DecodeError, "an independent manager must not see an uncommitted write")
	// The same write pool has no second writable connection.
	blockedCtx, blockedCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	blocked := writer.BeginTx(blockedCtx, nil)
	blockedCancel()
	require.ErrorIs(t, blocked.Error, context.DeadlineExceeded)
	require.Equal(t, 1, writer.DB().Stats().InUse)

	// Package-level writers in other managers/processes must also acquire the
	// same lock. Waiting for it observes the caller's cancellation.
	contender, err := acquireFileLock(ctx, datasetLockPath(meta), false)
	require.ErrorIs(t, err, ErrBusy)
	require.Nil(t, contender)
	blockedCtx, blockedCancel = context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = other.GetOrCreate(input, WithContext(blockedCtx), WithProtocols(true), WithFieldIndex("$.Questions"))
	blockedCancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)

	require.NoError(t, tx.Commit().Error, "WAL commit must finish while all four read snapshots are still open")
	for _, reader := range readers {
		var packet Packet
		require.NoError(t, reader.First(&packet, 1).Error)
		require.Empty(t, packet.DecodeError, "a read transaction retains its original snapshot")
		err := reader.Model(&Packet{}).Where("id = ?", 1).Update("decode_error", "reader write").Error
		var sqliteErr sqlite3.Error
		require.ErrorAs(t, err, &sqliteErr)
		require.Equal(t, sqlite3.ErrReadonly, sqliteErr.Code)
		require.NoError(t, reader.Commit().Error)
	}
	rows, err = fresh.QueryPackets(QueryContext(ctx))
	require.NoError(t, err)
	require.Equal(t, "committed by writer", rows[0].DecodeError)
	rows, err = db.QueryPackets(QueryContext(ctx))
	require.NoError(t, err)
	require.Equal(t, "committed by writer", rows[0].DecodeError)
	// Checkpoint on the writer after reader gaps, and verify the data remains
	// valid. Only the serialized writer runs checkpoints; public readers expose
	// typed queries and close their Rows instead of retaining snapshots.
	var busy, frames, copied int
	require.NoError(t, writer.Raw("PRAGMA wal_checkpoint(PASSIVE)").Row().Scan(&busy, &frames, &copied))
	require.Zero(t, busy)
	require.Equal(t, frames, copied)
	require.NoError(t, validateIndexContent(ctx, db.db, meta))
}

func TestPCAPDBConcurrentQueriesAndFieldIndexBuild(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 8))
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := make(chan struct{})
	var workers sync.WaitGroup
	failures := make(chan error, 25)
	for i := 0; i < 24; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for j := 0; j < 10; j++ {
				packets, err := db.QueryPackets(QueryContext(ctx))
				if err != nil || len(packets) != 8 {
					failures <- fmt.Errorf("packet query: count=%d, error=%v", len(packets), err)
					return
				}
				messages, err := db.QueryProtocols(QueryContext(ctx), QueryProtocol("dns"), QueryWithFields(true))
				if err != nil || len(messages) != 8 {
					failures <- fmt.Errorf("protocol query: count=%d, error=%v", len(messages), err)
					return
				}
				if _, err := db.ReadPacket(packets[j%8].ID); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		indexed, err := m.GetOrCreate(input, WithContext(ctx), WithProtocols(true), WithFieldIndex("$.ID"))
		if err != nil {
			failures <- err
		} else if indexed != db {
			failures <- errors.New("field-index build replaced the public database instance")
		}
	}()
	close(start)
	workers.Wait()
	close(failures)
	for failure := range failures {
		require.NoError(t, failure)
	}
	valid, err := m.Validate(ctx, db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid)
}

func TestPCAPDBWriterSettingsSurviveConnectionReplacement(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	writer.DB().SetMaxIdleConns(0) // each following PRAGMA needs a new connection
	for _, setting := range []struct {
		name string
		want int
	}{{"synchronous", 2}, {"fullfsync", 1}, {"foreign_keys", 1}} {
		var value int
		require.NoError(t, writer.Raw("PRAGMA "+setting.name).Row().Scan(&value))
		require.Equal(t, setting.want, value, setting.name)
	}
}
