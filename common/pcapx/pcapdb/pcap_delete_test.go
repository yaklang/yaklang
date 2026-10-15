package pcapdb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestPCAPDBDeleteLifecycle(t *testing.T) {
	m := testManager(t)
	capture := classicCapture(t, 2)
	input := writeCapture(t, capture)
	db, err := m.GetOrCreate(input)
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	// Two independent managers exercise the OS lifetime lock, not a shared map.
	other, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer other.Close()
	busy, err := other.Delete(context.Background(), db.ID)
	require.ErrorIs(t, err, ErrBusy)
	require.False(t, busy.Deleted)
	require.Equal(t, "busy", busy.Error.Code)
	require.FileExists(t, meta.DatabasePath)
	require.NoError(t, db.Close())
	handle, err := other.Acquire(context.Background(), db.ID)
	require.NoError(t, err)
	_, err = m.Delete(context.Background(), db.ID)
	require.ErrorIs(t, err, ErrBusy)
	rows, err := handle.QueryPackets()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NoError(t, handle.Close())
	writer, err := acquireFileLock(context.Background(), datasetLockPath(meta), false)
	require.NoError(t, err)
	_, err = m.Delete(context.Background(), db.ID)
	require.ErrorIs(t, err, ErrBusy)
	releaseFileLock(writer)
	// Also clean a rollback sidecar left by an interrupted SQLite transition.
	require.NoError(t, os.WriteFile(meta.DatabasePath+"-journal", []byte("fixture"), 0600))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.Delete(ctx, db.ID)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, checkNotDeleted(meta.DatabasePath))
	deleted, err := m.Delete(context.Background(), db.ID)
	require.NoError(t, err)
	require.True(t, deleted.Deleted)
	require.False(t, deleted.CleanupPending)
	require.Nil(t, deleted.Error)
	require.NoFileExists(t, meta.DatabasePath)
	require.NoFileExists(t, meta.DatabasePath+"-wal")
	require.NoFileExists(t, meta.DatabasePath+"-shm")
	require.NoFileExists(t, meta.DatabasePath+"-journal")
	require.FileExists(t, filepath.Join(filepath.Dir(meta.DatabasePath), deletionMarker))
	data, err := os.ReadFile(input)
	require.NoError(t, err)
	require.Equal(t, capture, data)
	count, err := m.Count()
	require.NoError(t, err)
	require.Zero(t, count)
	_, err = other.Open(context.Background(), db.ID)
	require.ErrorIs(t, err, ErrNotFound)
	// A caller retaining an old pool pointer cannot reopen it through acquisition.
	_, err = m.acquire(context.Background(), db)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = m.Delete(context.Background(), db.ID)
	require.NoError(t, err)
	_, err = m.Delete(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, ErrNotFound)
	for _, wrong := range []string{input, "1", "../" + db.ID} {
		_, err = m.Delete(context.Background(), wrong)
		require.Error(t, err)
	}
	// Soft deletion must release the unique full-content key for reimport.
	fresh, err := m.GetOrCreate(input)
	require.NoError(t, err)
	require.NotEqual(t, db.ID, fresh.ID)
	require.NoError(t, fresh.Close())
}

func TestPCAPDBDeleteWithReaderProcess(t *testing.T) {
	if os.Getenv("PCAPDB_DELETE_READER") == "1" {
		profile, err := gorm.Open("sqlite3", os.Getenv("PCAPDB_DELETE_PROFILE"))
		require.NoError(t, err)
		defer profile.Close()
		m, err := NewInstanceManager(profile, os.Getenv("PCAPDB_DELETE_ROOT"))
		require.NoError(t, err)
		defer m.Close()
		db, err := m.Open(context.Background(), os.Getenv("PCAPDB_DELETE_ID"))
		require.NoError(t, err)
		fmt.Println("PCAPDB_READER_READY")
		_, err = io.CopyN(io.Discard, os.Stdin, 1)
		require.NoError(t, err)
		rows, err := db.QueryPackets()
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.NoError(t, db.Close())
		return
	}
	profilePath := filepath.Join(t.TempDir(), "profile.sqlite")
	profile, err := gorm.Open("sqlite3", profilePath)
	require.NoError(t, err)
	defer profile.Close()
	m, err := NewInstanceManager(profile, t.TempDir())
	require.NoError(t, err)
	defer m.Close()
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPCAPDBDeleteWithReaderProcess$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "PCAPDB_DELETE_READER=1", "PCAPDB_DELETE_PROFILE="+profilePath, "PCAPDB_DELETE_ROOT="+m.root, "PCAPDB_DELETE_ID="+db.ID)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill() }()
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			if scanner.Text() == "PCAPDB_READER_READY" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		require.True(t, ok, "reader process did not become ready")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	result, err := m.Delete(ctx, db.ID)
	require.ErrorIs(t, err, ErrBusy)
	require.False(t, result.Deleted)
	_, err = in.Write([]byte{1})
	require.NoError(t, err)
	require.NoError(t, in.Close())
	require.NoError(t, cmd.Wait())
	result, err = m.Delete(ctx, db.ID)
	require.NoError(t, err)
	require.True(t, result.Deleted)
}

func TestPCAPDBDeleteRecovery(t *testing.T) {
	for _, catalogLost := range []bool{false, true} {
		t.Run(map[bool]string{false: "intent_before_catalog_commit", true: "intent_without_catalog"}[catalogLost], func(t *testing.T) {
			m := testManager(t)
			input := writeCapture(t, classicCapture(t, 2))
			db, err := m.GetOrCreate(input)
			require.NoError(t, err)
			meta, err := db.Metadata()
			require.NoError(t, err)
			require.NoError(t, db.Close())
			// Simulate a process dying after durable intent, before catalog/files.
			require.NoError(t, publishDeletion(filepath.Dir(meta.DatabasePath), db.ID))
			if catalogLost {
				require.NoError(t, m.profile.Unscoped().Where("dataset_id = ?", db.ID).Delete(&PCAPFileDBMetadata{}).Error)
			}
			require.NoError(t, m.Close())
			reopened, err := NewInstanceManager(m.profile, m.root)
			require.NoError(t, err)
			defer reopened.Close()
			count, err := reopened.Count()
			require.NoError(t, err)
			require.Zero(t, count)
			require.NoFileExists(t, meta.DatabasePath)
			require.FileExists(t, input)
			_, err = reopened.Open(context.Background(), db.ID)
			require.ErrorIs(t, err, ErrNotFound)
			// Even a later catalog-loss recovery cannot revive the deleted child.
			require.NoError(t, reopened.recoverCatalog())
			count, err = reopened.Count()
			require.NoError(t, err)
			require.Zero(t, count)
			fresh, err := reopened.GetOrCreate(input)
			require.NoError(t, err)
			require.NotEqual(t, db.ID, fresh.ID)
			require.NoError(t, fresh.Close())
		})
	}
}

func TestPCAPDBDeleteRejectsForeignPaths(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, db.Close())
	foreign := filepath.Join(t.TempDir(), "unrelated.sqlite")
	require.NoError(t, os.WriteFile(foreign, []byte("keep"), 0600))
	require.NoError(t, m.profile.Model(&PCAPFileDBMetadata{}).Where("dataset_id = ?", db.ID).UpdateColumn("database_path", foreign).Error)
	_, err = m.Delete(context.Background(), db.ID)
	require.Error(t, err)
	data, err := os.ReadFile(foreign)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
	require.FileExists(t, meta.DatabasePath)
}

func TestPCAPDBDeleteCleanupFailureIsRetryable(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	require.NoError(t, db.Close())
	// A nonempty directory in place of a sidecar forces physical cleanup to
	// fail after durable intent and catalog commit, without permission tricks.
	blocked := meta.DatabasePath + "-wal"
	require.NoError(t, os.Mkdir(blocked, 0700))
	sentinel := filepath.Join(blocked, "blocked")
	require.NoError(t, os.WriteFile(sentinel, []byte("fixture"), 0600))
	result, err := m.Delete(context.Background(), db.ID)
	require.Error(t, err)
	require.True(t, result.Deleted)
	require.True(t, result.CleanupPending)
	require.NotNil(t, result.Error)
	require.FileExists(t, meta.DatabasePath)
	_, err = m.Open(context.Background(), db.ID)
	require.ErrorIs(t, err, ErrNotFound)
	resumed, err := NewInstanceManager(m.profile, m.root)
	require.NoError(t, err)
	defer resumed.Close()
	require.NotEmpty(t, resumed.StartupValidation())
	count, err := resumed.Count()
	require.NoError(t, err)
	require.Zero(t, count)
	require.NoError(t, os.Remove(sentinel))
	require.NoError(t, os.Remove(blocked))
	result, err = resumed.Delete(context.Background(), db.ID)
	require.NoError(t, err)
	require.True(t, result.Deleted)
	require.False(t, result.CleanupPending)
	require.NoFileExists(t, meta.DatabasePath)
}
