package pcapdb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
)

const deletionMarker = ".deleted"

type DeleteResult struct {
	DatasetID      string       `json:"dataset_id"`
	Deleted        bool         `json:"deleted"`
	CleanupPending bool         `json:"cleanup_pending"`
	Error          *ResultError `json:"error"`
}

func checkNotDeleted(path string) error {
	_, err := os.Lstat(filepath.Join(filepath.Dir(path), deletionMarker))
	if err == nil {
		return ErrNotFound
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Reader pools hold a shared lifetime lock, including idle physical connections.
// Delete takes it exclusively, so it cannot unlink a SQLite/WAL file still open
// in another manager or process. This is independent of the analysis writer lock.
func openReaderPool(ctx context.Context, path string) (*gorm.DB, *os.File, error) {
	lock, err := acquireReadFileLock(ctx, filepath.Join(filepath.Dir(path), ".readers.lock"), false)
	if err != nil {
		return nil, nil, err
	}
	if err = checkNotDeleted(path); err != nil {
		releaseFileLock(lock)
		return nil, nil, err
	}
	reader, err := openIndex(path, false, false)
	if err != nil {
		releaseFileLock(lock)
		return nil, nil, err
	}
	return reader, lock, nil
}

func publishDeletion(directory, id string) error {
	path := filepath.Join(directory, deletionMarker)
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Size() != int64(len(id)+1) {
			return fmt.Errorf("pcapdb: invalid deletion marker")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != id+"\n" {
			return fmt.Errorf("pcapdb: deletion marker identity mismatch")
		}
		return syncDirectory(directory)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(directory, ".delete-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(id + "\n")
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

// Delete removes only package-owned child files and soft-deletes the catalog
// entry. Source captures and exports are never removed. Exact DatasetID is
// required; source paths, aliases and numeric catalog IDs are not accepted.
// A durable tombstone precedes cleanup. Retrying is idempotent, and startup
// resumes interrupted cleanup without recovering the child as a live dataset.
func (m *InstanceManager) Delete(ctx context.Context, id string) (result *DeleteResult, resultErr error) {
	result = &DeleteResult{DatasetID: id}
	defer func() { result.Error = resultError(resultErr) }()
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return result, fmt.Errorf("pcapdb: delete requires an exact canonical DatasetID")
	}
	if ctx == nil {
		return result, fmt.Errorf("pcapdb: nil deletion context")
	}
	if err = m.begin(); err != nil {
		return result, err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	directory := filepath.Join(m.root, id)
	path := filepath.Join(directory, "index.sqlite")
	var meta PCAPFileDBMetadata
	err = indexWithContext(ctx, m.profile).Unscoped().Where("dataset_id = ?", id).First(&meta).Error
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return result, err
	}
	if gorm.IsRecordNotFoundError(err) {
		if !errors.Is(checkNotDeleted(path), ErrNotFound) {
			return result, ErrNotFound
		}
	} else if meta.DatabasePath != path || meta.DatabaseType != "sqlite" {
		return result, fmt.Errorf("pcapdb: refusing to delete a child outside this library")
	}
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		err = makeDurableDirectory(directory)
	} else if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		err = fmt.Errorf("pcapdb: refusing a symlink or non-directory dataset")
	}
	if err != nil {
		return result, err
	}
	writer, err := acquireFileLock(ctx, filepath.Join(directory, ".instance.lock"), false)
	if err != nil {
		return result, err
	}
	defer releaseFileLock(writer)
	readers, err := acquireFileLock(ctx, filepath.Join(directory, ".readers.lock"), false)
	if err != nil {
		return result, err
	}
	defer releaseFileLock(readers)
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = publishDeletion(directory, id); err != nil {
		if errors.Is(checkNotDeleted(path), ErrNotFound) {
			result.Deleted, result.CleanupPending = true, true
		}
		return result, err
	}
	result.Deleted, result.CleanupPending = true, true
	// Once intent is durable, finish the small catalog transaction even if the
	// requesting task is cancelled. A process crash is handled on next startup.
	commitCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer finish()
	tx := m.profile.BeginTx(commitCtx, nil)
	if tx.Error != nil {
		return result, tx.Error
	}
	defer tx.Rollback()
	// The existing unique content index includes soft-deleted rows. Release its
	// key in this same transaction so a later import can obtain a new DatasetID.
	if err = indexWithContext(commitCtx, tx).Unscoped().Model(&PCAPFileDBMetadata{}).Where("dataset_id = ?", id).UpdateColumn("full_sha256", "").Error; err != nil {
		return result, err
	}
	if err = indexWithContext(commitCtx, tx).Where("dataset_id = ?", id).Delete(&PCAPFileDBMetadata{}).Error; err != nil {
		return result, err
	}
	if err = tx.Commit().Error; err != nil {
		return result, err
	}
	for _, name := range []string{"index.sqlite-wal", "index.sqlite-shm", "index.sqlite-journal", "index.sqlite"} {
		if err = os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	if err = syncDirectory(directory); err != nil {
		return result, err
	}
	// Lock files and the small tombstone stay: unlinking a lock would let an
	// existing waiter and a new opener lock different inodes for the same ID.
	result.CleanupPending = false
	return result, nil
}

func (m *InstanceManager) recoverDeletions() error {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return err
	}
	// Completed tombstones need no SQLite open, transaction or fsync at startup.
	// Load active identities once, then probe only the owned SQLite data files.
	var activeIDs []string
	if err = m.profile.Model(&PCAPFileDBMetadata{}).Pluck("dataset_id", &activeIDs).Error; err != nil {
		return err
	}
	active := make(map[string]bool, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = true
	}
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() {
			continue
		}
		if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
			continue
		}
		path := filepath.Join(m.root, id, "index.sqlite")
		if !errors.Is(checkNotDeleted(path), ErrNotFound) {
			continue
		}
		if !active[id] {
			clean := true
			for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
				if _, err := os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
					clean = false
					break
				}
			}
			if clean {
				continue
			}
		}
		if _, err = m.Delete(m.ctx, id); err != nil && !errors.Is(err, ErrBusy) {
			m.startup = append(m.startup, ValidationResult{Metadata: PCAPFileDBMetadata{DatasetID: id, DatabasePath: path}, Error: fmt.Sprintf("pcapdb: pending deletion: %v", err)})
		}
	}
	return nil
}
