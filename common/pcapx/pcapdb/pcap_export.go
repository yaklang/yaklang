package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/gorm"
)

// Export publishes an exact reconstruction of the stored PCAP/PCAPNG, including options,
// unknown blocks, interfaces and timestamp precision. It never overwrites an
// existing output. Subset/format-converting exports are outside this first batch.
func (m *InstanceManager) Export(ctx context.Context, identifier, output string) (string, error) {
	if err := m.begin(); err != nil {
		return "", err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	result, err := m.Validate(ctx, identifier, false)
	if err != nil {
		return "", err
	}
	if result.Busy {
		return "", ErrBusy
	}
	if !result.Valid {
		return "", fmt.Errorf("%w: %s", ErrNotReady, result.Error)
	}
	lock, err := acquireReadFileLock(ctx, datasetLockPath(&result.Metadata), false)
	if err != nil {
		return "", err
	}
	defer releaseFileLock(lock)
	reader, err := openIndex(result.Metadata.DatabasePath, false, false)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	meta, err := readManifest(ctx, reader, result.Metadata.DatasetID)
	if err != nil {
		return "", err
	}
	if meta.State != StateReady {
		return "", ErrNotReady
	}
	return exportCapture(ctx, reader, meta, output)
}

func (d *Database) Export(output string) (string, error) {
	d.mu.RLock()
	if d.closed {
		d.mu.RUnlock()
		return "", ErrClosed
	}
	d.mu.RUnlock()
	return d.manager.Export(context.Background(), d.ID, output)
}

func exportCapture(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata, output string) (string, error) {
	path, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(path); err == nil {
		return "", fmt.Errorf("pcapdb: output already exists: %w", os.ErrExist)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	source := newCaptureStoreReader(ctx, db, meta)
	defer source.Close()
	temp, err := os.CreateTemp(filepath.Dir(path), ".pcapdb-export-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	hash := sha256.New()
	n, err := copyContext(ctx, io.MultiWriter(temp, hash), source)
	if err == nil && (n != meta.SourceSize || hex.EncodeToString(hash.Sum(nil)) != meta.FullSHA256) {
		err = fmt.Errorf("pcapdb: managed capture content changed; export aborted")
	}
	if err == nil {
		err = temp.Sync()
	}
	err = errors.Join(err, temp.Close())
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	// Same-directory temporary file + exclusive link makes publication atomic
	// without Rename's overwrite behavior. A racing output creator wins safely.
	if err = os.Link(temp.Name(), path); err != nil {
		return "", err
	}
	if err = syncDirectory(filepath.Dir(path)); err != nil {
		return path, err
	}
	return path, nil
}
