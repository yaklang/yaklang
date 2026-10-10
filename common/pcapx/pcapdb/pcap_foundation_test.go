package pcapdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestPCAPDBCallerLeases(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 8))
	tools1 := ExportsWithManager(context.Background(), m)
	tools2 := ExportsWithManager(context.Background(), m)
	import1 := tools1["GetOrCreatePCAPDatabase"].(func(string, ...ImportOption) (*DatabaseHandle, error))
	open2 := tools2["OpenPCAPDatabase"].(func(string) (*DatabaseHandle, error))
	first, err := import1(input, WithProtocols(true))
	require.NoError(t, err)
	second, err := open2(first.ID)
	require.NoError(t, err)
	require.NotSame(t, first, second)
	require.Same(t, first.database, second.database)
	require.NoError(t, first.Close())
	_, err = first.ReadPacket(1)
	require.ErrorIs(t, err, ErrClosed)
	_, err = second.ReadPacket(1)
	require.NoError(t, err)
	third, err := tools1["OpenPCAPDatabase"].(func(string) (*DatabaseHandle, error))(first.ID)
	require.NoError(t, err)
	require.NoError(t, tools1["ClosePCAPDatabases"].(func() error)())
	_, err = third.QueryPackets()
	require.Error(t, err)
	_, err = second.QueryPackets()
	require.NoError(t, err, "one module must not close another execution's leases")
	rebuilt, err := second.RebuildAnalysis()
	require.NoError(t, err)
	require.Same(t, second, rebuilt, "instance rebuild must not leak an untracked lease")
	require.NoError(t, second.Close())
	m.mu.Lock()
	remaining := len(m.instances)
	m.mu.Unlock()
	require.Zero(t, remaining, "the final managed lease releases the cached read pool")
	// Repeated acquisitions must not exhaust the 32-dataset pool slots.
	for i := 0; i < 40; i++ {
		h, err := m.Acquire(context.Background(), first.ID)
		require.NoError(t, err)
		require.NoError(t, h.Close())
	}
}

func TestPCAPDBLeaseCancellationReachesDetailReads(t *testing.T) {
	m := testManager(t)
	native, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)), WithProtocols(true))
	require.NoError(t, err)
	methods := map[string]func(*DatabaseHandle) error{
		"packets": func(h *DatabaseHandle) error {
			_, err := h.QueryPackets(QueryContext(context.Background()))
			return err
		},
		"protocols": func(h *DatabaseHandle) error {
			_, err := h.QueryProtocols(QueryContext(context.Background()))
			return err
		},
		"packet BLOB": func(h *DatabaseHandle) error {
			_, err := h.ReadPacket(1, QueryContext(context.Background()))
			return err
		},
		"protocol BLOB": func(h *DatabaseHandle) error {
			_, err := h.ReadProtocol(1, QueryContext(context.Background()))
			return err
		},
		"protocol detail": func(h *DatabaseHandle) error {
			_, err := h.ProtocolDetails(1, QueryContext(context.Background()))
			return err
		},
		"sessions": func(h *DatabaseHandle) error {
			_, err := h.QuerySessions(QueryContext(context.Background()))
			return err
		},
		"stream": func(h *DatabaseHandle) error {
			_, err := h.ReadStream(1, QueryContext(context.Background()))
			return err
		},
		"discovery": func(h *DatabaseHandle) error {
			_, err := h.DiscoverProtocolFields(QueryContext(context.Background()))
			return err
		},
		"preview": func(h *DatabaseHandle) error {
			_, err := h.ReadPacketPage(1, QueryContext(context.Background()))
			return err
		},
	}
	for name, read := range methods {
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			h, err := m.Acquire(parent, native.ID)
			require.NoError(t, err)
			defer h.Close()
			// Pin every physical reader. The operation must wait for a connection;
			// cancellation has to reach SQLite rather than just an outer wrapper.
			var held []*sql.Tx
			for i := 0; i < 4; i++ {
				tx, err := native.db.DB().BeginTx(context.Background(), nil)
				require.NoError(t, err)
				held = append(held, tx)
			}
			defer func() {
				for _, tx := range held {
					_ = tx.Rollback()
				}
			}()
			before := native.db.DB().Stats().WaitCount
			done := make(chan error, 1)
			go func() { done <- read(h) }()
			require.Eventually(t, func() bool { return native.db.DB().Stats().WaitCount > before }, time.Second, time.Millisecond)
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("canceled SQL is still waiting for a reader")
			}
		})
	}
	// A native owner remains usable after task-owned leases are canceled.
	_, err = native.ReadPacket(1)
	require.NoError(t, err)
}

func TestPCAPDBLeasesConcurrentReleaseAndShutdown(t *testing.T) {
	m := testManager(t)
	native, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 4)))
	require.NoError(t, err)
	id := native.ID
	require.NoError(t, native.Close())
	var wg sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 6; j++ {
				h, err := m.Acquire(context.Background(), id)
				if err != nil {
					failures <- err
					return
				}
				if _, err = h.ReadPacket(1); err != nil {
					failures <- err
					return
				}
				if err = h.Close(); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	h, err := m.Acquire(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, m.Close())
	_, err = h.QueryPackets()
	require.True(t, errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled))
	require.NoError(t, h.Close())
}

func TestPCAPDBBoundedResultPages(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 40)), WithProtocols(true))
	require.NoError(t, err)
	// Escaping can expand text sixfold. Budget actual JSON, not input lengths.
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).UpdateColumn("summary", strings.Repeat("\x01", 1600)).Error)
	require.NoError(t, writer.Close())
	var after int64
	var seen []uint
	for {
		page, err := db.QueryProtocolsPage(QueryAfter(after), QueryLimit(40), QueryResultBytes(4096))
		require.NoError(t, err)
		require.LessOrEqual(t, mustJSONSize(t, page), 4096)
		require.Equal(t, StateReady, page.State)
		require.Equal(t, db.ID, page.DatasetID)
		require.Nil(t, page.Error)
		require.NotEmpty(t, page.Items)
		for _, item := range page.Items {
			seen = append(seen, item.ID)
			require.True(t, item.PreviewTruncated)
		}
		require.Greater(t, page.NextCursor, after)
		if !page.HasMore {
			break
		}
		after = page.NextCursor
	}
	require.Len(t, seen, 40)
	for i, id := range seen {
		require.EqualValues(t, i+1, id, "byte-limited pagination must not skip records")
	}
	page, err := db.QueryPacketsPage(QueryLimit(2))
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.True(t, page.HasMore)
	last, err := db.QueryPacketsPage(QueryAfter(40))
	require.NoError(t, err)
	require.Empty(t, last.Items)
	require.False(t, last.HasMore)
	require.EqualValues(t, 40, last.NextCursor)
	catalog, err := m.ListPage(context.Background(), QueryLimit(1))
	require.NoError(t, err)
	require.Len(t, catalog.Items, 1)
	require.False(t, catalog.HasMore)
	preview, err := db.ReadPacketPage(1, QueryPreviewBytes(5))
	require.NoError(t, err)
	require.Len(t, preview.Items[0].Data, 5)
	require.True(t, preview.Items[0].Truncated)
	empty, err := db.ReadProtocolPage(1, QueryPreviewBytes(0))
	require.NoError(t, err)
	require.Empty(t, empty.Items[0].Data)
	require.True(t, empty.Items[0].Truncated)
	_, err = db.QueryPacketsPage(QueryResultBytes(100))
	require.Error(t, err)
	failure, err := db.ReadPacketPage(9999)
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, "not_found", failure.Error.Code)
	require.Empty(t, failure.Items)
	// A 64-KiB preview won't fit into 4 KiB after base64 expansion.
	writer, err = openIndex(db.path, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&Packet{}).Where("id = ?", 1).Updates(map[string]any{"data": []byte(strings.Repeat("x", 9000)), "captured_length": 9000}).Error)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", 1).UpdateColumn("fields", gorm.Expr("jsonb(?)", `{"large":"`+strings.Repeat("x", 9000)+`"}`)).Error)
	require.NoError(t, writer.Close())
	tooLarge, err := db.ReadPacketPage(1, QueryPreviewBytes(9000), QueryResultBytes(4096))
	require.ErrorIs(t, err, ErrResultTooLarge)
	require.Equal(t, "result_too_large", tooLarge.Error.Code)
	require.Zero(t, tooLarge.NextCursor)
	detail, err := db.ProtocolDetailsPage(1, QueryResultBytes(4096))
	require.ErrorIs(t, err, ErrResultTooLarge)
	require.Empty(t, detail.Items)
}

func mustJSONSize(t *testing.T, value any) int {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return len(data)
}

func TestPCAPDBFieldDiscoveryAndIndexByDatasetID(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 64))
	db, err := m.GetOrCreate(input, WithProtocols(true))
	require.NoError(t, err)
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).UpdateColumn("fields", gorm.Expr("jsonb(?)", `{"v":1}`)).Error)
	fields := []string{`{"v":null,"header":{"x.y":"abc"},"array":[{"big":9007199254740993}]}`, `{"v":true,"header":{"x.y":false}}`, `{"v":1}`}
	for i, value := range fields {
		require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", i+1).UpdateColumn("fields", gorm.Expr("jsonb(?)", value)).Error)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.Remove(input))
	before, err := db.Metadata()
	require.NoError(t, err)
	discovery, err := db.DiscoverProtocolFields(QueryProtocol("dns"), QueryLimit(2), QueryResultBytes(4096))
	require.NoError(t, err)
	require.True(t, discovery.Sampled)
	require.Equal(t, 2, discovery.Scanned)
	require.True(t, discovery.HasMore)
	require.EqualValues(t, 2, discovery.NextCursor)
	byPath := map[string]ProtocolField{}
	for _, field := range discovery.Items {
		byPath[field.Path] = field
	}
	require.ElementsMatch(t, []string{"null", "true"}, byPath["$.v"].Types)
	path := `$.header."x.y"`
	require.True(t, byPath[path].Searchable)
	require.False(t, byPath[path].Indexed)
	require.Contains(t, byPath, "$.array[0].big")
	indexes, err := m.EnsureProtocolFieldIndexes(context.Background(), db.ID, path, "$.array[0].big")
	require.NoError(t, err)
	require.Len(t, indexes, 2)
	again, err := m.EnsureProtocolFieldIndexes(context.Background(), db.ID, path)
	require.NoError(t, err)
	require.Equal(t, indexes, again)
	assertProtocolFieldQueryPlan(t, db.db, path, "abc")
	rows, err := db.QueryProtocols(QueryField("$.array[0].big", json.Number("9007199254740993")))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	after, err := db.Metadata()
	require.NoError(t, err)
	require.Equal(t, before.State, after.State)
	require.Equal(t, before.PacketCount, after.PacketCount)
	require.Equal(t, before.ProtocolDataSHA256, after.ProtocolDataSHA256)
	discovery, err = db.DiscoverProtocolFields(QueryAfter(63), QueryLimit(2))
	require.NoError(t, err)
	require.False(t, discovery.HasMore)
	require.Equal(t, 1, discovery.Scanned)
	discovery, err = db.DiscoverProtocolFields()
	require.NoError(t, err)
	for _, field := range discovery.Items {
		if field.Path == path {
			require.True(t, field.Indexed)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.EnsureProtocolFieldIndexes(canceled, db.ID, "$.cancelled")
	require.ErrorIs(t, err, context.Canceled)
	inventory, err := db.ProtocolFieldIndexes()
	require.NoError(t, err)
	require.Len(t, inventory, 2)
	// Public index creation shares the all-or-nothing 16-index guard.
	var paths []string
	for i := 0; i < 13; i++ {
		paths = append(paths, fmt.Sprintf("$.f%d", i))
	}
	_, err = m.EnsureProtocolFieldIndexes(context.Background(), db.ID, paths...)
	require.NoError(t, err)
	_, err = m.EnsureProtocolFieldIndexes(context.Background(), db.ID, "$.last", "$.overflow")
	require.Error(t, err)
	inventory, err = db.ProtocolFieldIndexes()
	require.NoError(t, err)
	require.Len(t, inventory, 15)
	for _, index := range inventory {
		require.NotEqual(t, "$.last", index.Path)
	}
}

func TestPCAPDBFieldDiscoveryBudgets(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)), WithProtocols(true))
	require.NoError(t, err)
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	wide := `{"a":[` + strings.Repeat(`1,`, discoveryNodes) + `1]}`
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", 1).UpdateColumn("fields", gorm.Expr("jsonb(?)", wide)).Error)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", 2).UpdateColumn("fields", gorm.Expr("jsonb(?)", `{"leaf":null}`)).Error)
	require.NoError(t, writer.Close())
	page, err := db.DiscoverProtocolFields(QueryResultBytes(4096))
	require.NoError(t, err)
	require.Equal(t, 1, page.Skipped)
	require.True(t, page.Truncated)
	require.EqualValues(t, 2, page.NextCursor)
	require.False(t, page.HasMore)
	require.Len(t, page.Items, 1)
	require.Equal(t, "$.leaf", page.Items[0].Path)
	require.LessOrEqual(t, mustJSONSize(t, page), 4096)
}

func TestPCAPDBPayloadArtifacts(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)), WithProtocols(true))
	require.NoError(t, err)
	dir := t.TempDir()
	artifact, err := db.ExportPacket(1, filepath.Join(dir, "packet.bin"))
	require.NoError(t, err)
	data, err := os.ReadFile(artifact.Path)
	require.NoError(t, err)
	original, err := db.ReadPacket(1)
	require.NoError(t, err)
	require.Equal(t, original, data)
	require.EqualValues(t, len(data), artifact.Bytes)
	_, err = db.ExportPacket(2, artifact.Path)
	require.ErrorIs(t, err, os.ErrExist)
	streams, err := db.QueryStreams()
	require.NoError(t, err)
	require.NotEmpty(t, streams)
	stream, err := db.ExportStream(streams[0].ID, filepath.Join(dir, "stream.bin"))
	require.NoError(t, err)
	data, err = os.ReadFile(stream.Path)
	require.NoError(t, err)
	chunks, err := db.ReadStream(streams[0].ID)
	require.NoError(t, err)
	var expected []byte
	for _, chunk := range chunks.Chunks {
		expected = append(expected, chunk.Data...)
	}
	require.Equal(t, expected, data)
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&PCAPStreamChunk{}).Where("id = ?", chunks.Chunks[0].ID).UpdateColumn("references_complete", false).Error)
	require.NoError(t, writer.Close())
	previews, err := db.ReadStreamPage(streams[0].ID, QueryPreviewBytes(4), QueryResultBytes(4096))
	require.NoError(t, err)
	require.LessOrEqual(t, mustJSONSize(t, previews), 4096)
	require.False(t, *previews.Items[0].ReferencesComplete)
	if len(previews.Items) > 1 {
		require.True(t, *previews.Items[1].ReferencesComplete)
	}
	fieldArtifact, err := db.ExportProtocolFields(1, filepath.Join(dir, "fields.json"))
	require.NoError(t, err)
	data, err = os.ReadFile(fieldArtifact.Path)
	require.NoError(t, err)
	require.True(t, json.Valid(data))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target := filepath.Join(dir, "canceled.bin")
	_, err = db.ExportProtocol(1, target, QueryContext(ctx))
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(target)
	require.ErrorIs(t, err, os.ErrNotExist)
	temps, err := filepath.Glob(filepath.Join(dir, ".pcapdb-payload-*"))
	require.NoError(t, err)
	require.Empty(t, temps)
}

func TestPCAPDBLeaseDeadlineAndReaderLock(t *testing.T) {
	m := testManager(t)
	native, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	h, err := m.Acquire(parent, native.ID)
	require.NoError(t, err)
	defer h.Close()
	native.mu.Lock()
	done := make(chan error, 1)
	go func() { _, err := h.ReadPacket(1, QueryContext(context.Background())); done <- err }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		native.mu.Unlock()
		t.Fatal("task deadline cannot escape a reader refresh lock")
	}
	native.mu.Unlock()
	failed, err := h.QueryPacketsPage(QueryAfter(7))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "deadline_exceeded", failed.Error.Code)
	require.EqualValues(t, 7, failed.NextCursor)
}
