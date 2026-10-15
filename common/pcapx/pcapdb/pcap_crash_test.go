package pcapdb

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

type crashCheckpoint struct {
	Committed PCAPFileDBMetadata
	Pending   *PCAPFileDBMetadata
}

// This helper runs only in a child copy of the test binary. Process.Kill skips
// all Go defers, GORM Rollback/Close and manager failure publication. No crash
// hooks are added to production code and no user database is touched.
func TestPCAPDBCrashHelperProcess(t *testing.T) {
	scenario := os.Getenv("YAK_PCAPDB_CRASH_SCENARIO")
	if scenario == "" {
		return
	}
	profile, err := gorm.Open("sqlite3", os.Getenv("YAK_PCAPDB_CRASH_PROFILE"))
	require.NoError(t, err)
	profile.DB().SetMaxOpenConns(1)
	defer profile.Close()
	m, err := NewInstanceManager(profile, os.Getenv("YAK_PCAPDB_CRASH_ROOT"))
	require.NoError(t, err)
	defer m.Close()
	input := os.Getenv("YAK_PCAPDB_CRASH_INPUT")

	pause := func(id string, pending *PCAPFileDBMetadata) {
		catalog, err := m.Metadata(id)
		require.NoError(t, err)
		reader, err := openIndex(catalog.DatabasePath, false, false)
		require.NoError(t, err)
		committed, err := readManifest(context.Background(), reader, id)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		data, err := json.Marshal(crashCheckpoint{Committed: *committed, Pending: pending})
		require.NoError(t, err)
		marker := os.Getenv("YAK_PCAPDB_CRASH_MARKER")
		require.NoError(t, os.WriteFile(marker+".tmp", data, 0600))
		require.NoError(t, os.Rename(marker+".tmp", marker))
		<-time.NewTimer(time.Minute).C
		t.Fatal("parent did not kill the child at its crash checkpoint")
	}

	if scenario == "retry_reset_transaction" {
		ctx, cancel := context.WithCancel(context.Background())
		_, err = m.GetOrCreate(input, WithContext(ctx), WithBatchSize(2), WithProgress(func(p Progress) {
			if p.State == StateIndexing && p.PacketCount == 2 {
				cancel()
			}
		}))
		cancel()
		require.ErrorIs(t, err, context.Canceled)
	}
	if scenario == "packet_transaction" || scenario == "protocol_transaction" || scenario == "retry_reset_transaction" || scenario == "stream_transaction" || scenario == "session_transaction" {
		matches := func(meta *PCAPFileDBMetadata) bool {
			switch scenario {
			case "packet_transaction":
				return meta.State == StateIndexing && meta.PacketCount == 4
			case "protocol_transaction":
				return meta.State == StateAnalyzing && meta.ProtocolCount == 2
			case "stream_transaction":
				return meta.State == StateAnalyzing && meta.StreamChunkCount == 2
			case "session_transaction":
				return meta.State == StateAnalyzing && meta.SessionCount == 1 && meta.StreamChunkCount == 1
			default:
				return meta.State == StateIndexing && meta.PacketCount == 0
			}
		}
		manifest := func(scope *gorm.Scope) *PCAPFileDBMetadata {
			record, ok := scope.Value.(*PCAPDatasetInfo)
			if !ok {
				return nil
			}
			var meta PCAPFileDBMetadata
			require.NoError(t, json.Unmarshal([]byte(record.Metadata), &meta))
			if !matches(&meta) {
				return nil
			}
			return &meta
		}
		// A tiny cache forces dirty pages of the pending transaction into WAL,
		// so the torn-tail test exercises recovery of actual uncommitted frames.
		gorm.DefaultCallback.Create().Before("gorm:create").Register("pcapdb_test_small_cache", func(scope *gorm.Scope) {
			if manifest(scope) != nil {
				require.NoError(t, scope.DB().Exec("PRAGMA cache_size=1").Error)
				require.NoError(t, scope.DB().Exec("PRAGMA cache_spill=1").Error)
			}
		})
		gorm.DefaultCallback.Create().After("gorm:create").Before("gorm:commit_or_rollback_transaction").Register("pcapdb_test_crash", func(scope *gorm.Scope) {
			if meta := manifest(scope); meta != nil {
				var packets, messages int64
				require.NoError(t, scope.DB().Model(&Packet{}).Count(&packets).Error)
				require.NoError(t, scope.DB().Model(&PCAPProtocolMessage{}).Count(&messages).Error)
				require.Equal(t, meta.PacketCount, packets)
				require.Equal(t, meta.ProtocolCount, messages)
				pause(meta.DatasetID, meta)
			}
		})
	}
	protocols := scenario == "protocol_transaction" || scenario == "ready" || scenario == "field_index_transaction"
	batch := 2
	if protocols || scenario == "stream_transaction" || scenario == "session_transaction" {
		batch = 1
	}
	db, err := m.GetOrCreate(input, WithBatchSize(batch), WithProtocols(protocols), WithProgress(func(p Progress) {
		if (scenario == "packet_batch" && p.State == StateIndexing && p.PacketCount == 2) || (scenario == "ready" && p.State == StateReady) {
			pause(p.DatasetID, nil)
		}
	}))
	require.NoError(t, err)
	if scenario != "field_index_transaction" {
		t.Fatal("import unexpectedly passed the crash checkpoint")
	}
	meta, err := db.Metadata()
	require.NoError(t, err)
	lock, err := acquireFileLock(context.Background(), datasetLockPath(meta), true)
	require.NoError(t, err)
	defer releaseFileLock(lock)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	tx := writer.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	for _, path := range []string{"$.ID", "$.Questions[0].Name"} {
		require.NoError(t, tx.Model(&PCAPProtocolMessage{}).
			Where("deleted_at IS NULL AND "+protocolFieldScalarCondition(path)).
			AddIndex(protocolFieldIndexName(path), protocolFieldExpression("json_extract", path), protocolFieldExpression("json_type", path), "id").Error)
	}
	pause(db.ID, nil) // CREATE INDEX has run, but its transaction has not committed
}

type crashProcess struct {
	command *exec.Cmd
	done    <-chan error
	root    string
	profile string
	input   string
	marker  crashCheckpoint
	waited  bool
}

func startCrashProcess(t *testing.T, scenario string) *crashProcess {
	t.Helper()
	directory := t.TempDir()
	child := &crashProcess{
		root:    filepath.Join(directory, "library"),
		profile: filepath.Join(directory, "profile.sqlite"),
		input:   writeCapture(t, classicCapture(t, 7)),
	}
	marker := filepath.Join(directory, "checkpoint.json")
	executable, err := os.Executable()
	require.NoError(t, err)
	child.command = exec.Command(executable, "-test.run=^TestPCAPDBCrashHelperProcess$", "-test.timeout=50s")
	child.command.Env = append(os.Environ(),
		"YAK_PCAPDB_CRASH_SCENARIO="+scenario,
		"YAK_PCAPDB_CRASH_ROOT="+child.root,
		"YAK_PCAPDB_CRASH_PROFILE="+child.profile,
		"YAK_PCAPDB_CRASH_INPUT="+child.input,
		"YAK_PCAPDB_CRASH_MARKER="+marker)
	log, err := os.Create(filepath.Join(directory, "child.log"))
	require.NoError(t, err)
	defer log.Close()
	child.command.Stdout, child.command.Stderr = log, log
	require.NoError(t, child.command.Start())
	done := make(chan error, 1)
	child.done = done
	go func() { done <- child.command.Wait() }()
	t.Cleanup(func() {
		if !child.waited {
			child.command.Process.Kill()
			<-done
			child.waited = true
		}
	})
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			child.waited = true
			output, _ := os.ReadFile(log.Name())
			t.Fatalf("child exited before crash checkpoint: %v\n%s", err, output)
		case <-timer.C:
			output, _ := os.ReadFile(log.Name())
			t.Fatalf("child did not reach crash checkpoint\n%s", output)
		case <-ticker.C:
			data, err := os.ReadFile(marker)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &child.marker))
			return child
		}
	}
}

func (child *crashProcess) kill(t *testing.T) {
	t.Helper()
	require.NoError(t, child.command.Process.Kill())
	select {
	case err := <-child.done:
		child.waited = true
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		require.False(t, child.command.ProcessState.Success())
	case <-time.After(10 * time.Second):
		t.Fatal("child process did not terminate")
	}
}

func (child *crashProcess) reopen(t *testing.T) *InstanceManager {
	t.Helper()
	profile, err := gorm.Open("sqlite3", child.profile)
	require.NoError(t, err)
	profile.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, profile.Close()) })
	m, err := NewInstanceManager(profile, child.root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	return m
}

func assertCrashCheckpoint(t *testing.T, m *InstanceManager, before *PCAPFileDBMetadata, state State) *PCAPFileDBMetadata {
	t.Helper()
	meta, err := m.Metadata(before.DatasetID)
	require.NoError(t, err)
	require.Equal(t, state, meta.State)
	require.Equal(t, before.PacketCount, meta.PacketCount)
	require.Equal(t, before.ProtocolCount, meta.ProtocolCount)
	require.Equal(t, before.BytesIndexed, meta.BytesIndexed)
	require.Equal(t, before.ProtocolDataSize, meta.ProtocolDataSize)
	reader, err := openIndex(meta.DatabasePath, false, false)
	require.NoError(t, err)
	defer reader.Close()
	require.NoError(t, validateIndexContent(context.Background(), reader, meta))
	var integrity string
	require.NoError(t, reader.Raw("PRAGMA quick_check").Row().Scan(&integrity))
	require.Equal(t, "ok", integrity)
	return meta
}

func assertCrashRetry(t *testing.T, child *crashProcess, m *InstanceManager) {
	t.Helper()
	db, err := m.GetOrCreate(child.input, WithProtocols(true), WithBatchSize(2))
	require.NoError(t, err)
	require.Equal(t, child.marker.Committed.DatasetID, db.ID)
	packets, err := db.QueryPackets()
	require.NoError(t, err)
	require.Len(t, packets, 7)
	messages, err := db.QueryProtocols(QueryProtocol("dns"), QueryWithFields(true))
	require.NoError(t, err)
	require.Len(t, messages, 7)
	for i, message := range messages {
		require.EqualValues(t, i+1, message.ID, "retry must not duplicate partially imported rows")
		require.NotEmpty(t, message.Fields)
		_, err = db.ReadProtocol(message.ID)
		require.NoError(t, err)
	}
	valid, err := m.Validate(context.Background(), db.ID, true)
	require.NoError(t, err)
	require.True(t, valid.Valid, valid.Error)
	output := filepath.Join(t.TempDir(), "recovered.pcap")
	_, err = db.Export(output)
	require.NoError(t, err)
	original, err := os.ReadFile(child.input)
	require.NoError(t, err)
	exported, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, original, exported)
	count, err := m.Count()
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestPCAPDBProcessKillRecovery(t *testing.T) {
	for _, scenario := range []string{"packet_batch", "packet_transaction", "retry_reset_transaction", "protocol_transaction", "stream_transaction", "session_transaction", "ready", "field_index_transaction"} {
		t.Run(scenario, func(t *testing.T) {
			child := startCrashProcess(t, scenario)
			before := &child.marker.Committed
			wal, err := os.Stat(before.DatabasePath + "-wal")
			require.NoError(t, err)
			require.Greater(t, wal.Size(), int64(32), "exercise live WAL, not a gracefully closed database")
			if child.marker.Pending != nil {
				require.NotEqual(t, before.PacketCount+before.ProtocolCount+before.SessionCount+before.StreamChunkCount, child.marker.Pending.PacketCount+child.marker.Pending.ProtocolCount+child.marker.Pending.SessionCount+child.marker.Pending.StreamChunkCount)
			}
			if scenario == "protocol_transaction" {
				reader, err := openIndex(before.DatabasePath, false, false)
				require.NoError(t, err)
				var count int64
				require.NoError(t, reader.Model(&PCAPProtocolMessage{}).Count(&count).Error)
				require.Equal(t, before.ProtocolCount, count, "uncommitted message BLOBs must be invisible to other readers")
				require.NoError(t, reader.Close())
			}
			if scenario == "packet_batch" {
				profile, err := gorm.Open("sqlite3", child.profile)
				require.NoError(t, err)
				observer, err := NewInstanceManager(profile, child.root)
				require.NoError(t, err)
				require.Len(t, observer.StartupValidation(), 1)
				require.True(t, observer.StartupValidation()[0].Busy, "a different process must respect a live importer's lock")
				require.NoError(t, observer.Close())
				require.NoError(t, profile.Close())
			}
			child.kill(t)
			m := child.reopen(t)
			state := StateInterrupted
			if scenario == "ready" || scenario == "field_index_transaction" {
				state = StateReady
			}
			meta := assertCrashCheckpoint(t, m, before, state)
			if scenario == "protocol_transaction" {
				reader, err := openIndex(meta.DatabasePath, false, false)
				require.NoError(t, err)
				var size int64
				require.NoError(t, reader.Model(&PCAPProtocolMessage{}).Select("COALESCE(sum(length(data)),0)").Row().Scan(&size))
				require.Equal(t, meta.ProtocolDataSize, size, "message BLOBs and checkpoint recover atomically")
				require.NoError(t, reader.Close())
			}
			if state == StateInterrupted {
				_, err = m.Open(context.Background(), meta.DatasetID)
				require.ErrorIs(t, err, ErrNotReady)
			}
			if scenario == "field_index_transaction" {
				db, err := m.Open(context.Background(), meta.DatasetID)
				require.NoError(t, err)
				for _, path := range []string{"$.ID", "$.Questions[0].Name"} {
					var indexes int64
					require.NoError(t, db.db.Table("sqlite_schema").Where("type = ? AND name = ?", "index", protocolFieldIndexName(path)).Count(&indexes).Error)
					require.Zero(t, indexes, "uncommitted indexes must not survive process kill")
				}
				_, err = m.GetOrCreate(child.input, WithProtocols(true), WithFieldIndex("$.ID"), WithFieldIndex("$.Questions[0].Name"))
				require.NoError(t, err)
			}
			assertCrashRetry(t, child, m)
		})
	}
}

func TestPCAPDBRecoveryFromTornUncommittedWALFrame(t *testing.T) {
	child := startCrashProcess(t, "packet_transaction")
	child.kill(t)
	meta := &child.marker.Committed
	path := meta.DatabasePath + "-wal"
	wal, err := os.ReadFile(path)
	require.NoError(t, err)
	pageSize := int(binary.BigEndian.Uint32(wal[8:12]))
	frameSize := 24 + pageSize
	lastCommit := 32
	for offset := 32; offset+frameSize <= len(wal); offset += frameSize {
		if binary.BigEndian.Uint32(wal[offset+4:offset+8]) != 0 {
			lastCommit = offset + frameSize
		}
	}
	require.Greater(t, len(wal)-lastCommit, 0, "the helper must have spilled actual uncommitted WAL frames")
	// Damage only the final, uncommitted frame. Previously committed bytes are
	// retained. This is a file-level torn-write simulation, not a power-cut test.
	require.NoError(t, os.Truncate(path, int64(len(wal)-pageSize/2)))
	// With all processes dead, discard the volatile WAL-index so SQLite must
	// reconstruct it from WAL checksums and commit markers on startup.
	require.NoError(t, os.Remove(meta.DatabasePath+"-shm"))
	m := child.reopen(t)
	assertCrashCheckpoint(t, m, meta, StateInterrupted)
	assertCrashRetry(t, child, m)
}

func TestPCAPDBKilledReadyDatasetRecoversLostCatalog(t *testing.T) {
	child := startCrashProcess(t, "ready")
	child.kill(t)
	profile, err := gorm.Open("sqlite3", child.profile)
	require.NoError(t, err)
	// The profile mirror can lose a registration independently of a durable
	// child checkpoint. Simulate that loss without damaging the child files.
	require.NoError(t, profile.Unscoped().Where("dataset_id = ?", child.marker.Committed.DatasetID).Delete(&PCAPFileDBMetadata{}).Error)
	require.NoError(t, profile.Close())
	require.NoError(t, os.Remove(child.marker.Committed.DatabasePath+"-shm"))
	m := child.reopen(t)
	assertCrashCheckpoint(t, m, &child.marker.Committed, StateReady)
	assertCrashRetry(t, child, m)
}

func TestPCAPDBBLOBRecoveryPreservesCommittedDamage(t *testing.T) {
	child := startCrashProcess(t, "protocol_transaction")
	child.kill(t)
	meta := &child.marker.Committed
	m := child.reopen(t)
	assertCrashCheckpoint(t, m, meta, StateInterrupted)
	assertCrashRetry(t, child, m)
	ready, err := m.Metadata(meta.DatasetID)
	require.NoError(t, err)
	writer, err := openIndex(ready.DatabasePath, true, false)
	require.NoError(t, err)
	var record PCAPProtocolMessage
	require.NoError(t, writer.First(&record).Error)
	damaged := append([]byte{}, record.Data[:len(record.Data)-1]...)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", record.ID).UpdateColumn("data", damaged).Error)
	require.NoError(t, writer.Close())
	validation, err := m.Validate(context.Background(), meta.DatasetID, true)
	require.NoError(t, err)
	require.False(t, validation.Valid)
	require.Equal(t, StateInvalid, validation.Metadata.State)
	reader, err := openIndex(ready.DatabasePath, false, false)
	require.NoError(t, err)
	require.NoError(t, reader.First(&record).Error)
	require.Equal(t, damaged, record.Data, "validation must preserve committed damage until explicit reimport")
	require.NoError(t, reader.Close())
	assertCrashRetry(t, child, m)
}
