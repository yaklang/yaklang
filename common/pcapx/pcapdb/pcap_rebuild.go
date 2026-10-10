package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// RebuildAnalysis repairs derived protocol/session/stream data using packet
// BLOBs already in the child. It works without the original input, verifies the
// complete stored capture first, and never tries to repair damaged source bytes.
func (m *InstanceManager) RebuildAnalysis(identifier string, options ...ImportOption) (_ *Database, resultErr error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	config := &importConfig{ctx: context.Background(), batchSize: 2000, streams: true}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("pcapdb: nil rebuild option")
		}
		if err := option(config); err != nil {
			return nil, err
		}
	}
	ctx, cancel := m.operationContext(config.ctx)
	defer cancel()
	config.ctx, config.cancel = ctx, cancel
	if config.parent != nil {
		stop := context.AfterFunc(config.parent, cancel)
		defer stop()
		if config.parent.Err() != nil {
			cancel()
		}
	}
	catalog, err := m.resolve(identifier)
	if err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(ctx, datasetLockPath(catalog), true)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lock)
	reader, err := openIndex(catalog.DatabasePath, false, false)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	meta, err := readManifest(ctx, reader, catalog.DatasetID)
	if err != nil {
		return nil, err
	}
	if meta.DatabasePath != catalog.DatabasePath {
		return nil, fmt.Errorf("pcapdb: manifest/catalog paths disagree")
	}
	if meta.BytesIndexed != meta.SourceSize || len(meta.FullSHA256) != 64 || meta.CaptureRecordCount == 0 {
		return nil, fmt.Errorf("pcapdb: capture import is incomplete; reimport the original file")
	}
	config.protocols = config.protocols || meta.ProtocolsIndexed
	config.streams = config.streams || meta.StreamsIndexed
	if len(config.fieldIndexes) > 0 && !config.protocols {
		return nil, fmt.Errorf("pcapdb: field indexes require withProtocols(true)")
	}
	if !config.protocols && !config.streams {
		return nil, fmt.Errorf("pcapdb: no analysis selected for rebuild")
	}
	capture := newCaptureStoreReader(ctx, reader, meta)
	hash := sha256.New()
	_, hashErr := copyContext(ctx, hash, capture)
	capture.Close()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if hashErr == nil && hex.EncodeToString(hash.Sum(nil)) != meta.FullSHA256 {
		hashErr = fmt.Errorf("pcapdb: capture BLOB hash mismatch; source bytes cannot be repaired from derived data")
	}
	writer, err := openIndex(meta.DatabasePath, true, false)
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	if hashErr != nil {
		meta.State, meta.LastError = StateInvalid, hashErr.Error()
		return nil, errors.Join(hashErr, m.checkpoint(ctx, writer, meta))
	}
	defer func() {
		if resultErr != nil {
			resultErr = m.failImport(writer, meta, resultErr, config)
		}
	}()
	if meta.State != StateReady && meta.State != StateAnalyzing {
		if err = m.transition(ctx, writer, meta, StateIndexing); err != nil {
			return nil, err
		}
	}
	if err = m.indexProtocols(ctx, writer, meta, config); err != nil {
		return nil, err
	}
	if err = checkpointIndex(ctx, writer); err != nil {
		return nil, err
	}
	if len(config.fieldIndexes) > 0 {
		if err = m.refreshReader(ctx, meta); err != nil {
			return nil, err
		}
	}
	config.notify(meta)
	return m.openReady(meta, config.leased)
}

func (d *Database) RebuildAnalysis(options ...ImportOption) (*Database, error) {
	d.mu.RLock()
	closed := d.closed
	d.mu.RUnlock()
	if closed {
		return nil, ErrClosed
	}
	return d.manager.RebuildAnalysis(d.ID, options...)
}
