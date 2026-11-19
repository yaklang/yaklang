package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type importConfig struct {
	ctx               context.Context
	parent            context.Context
	batchSize         int
	protocols         bool
	streams           bool
	fieldIndexes      []string
	progress          func(Progress)
	cancel            context.CancelFunc
	progressErr       error
	leased            bool
	tlsSecrets        pcaputil.TLSSecretProvider
	tlsKeyLogSHA256   string
	deferQueryIndexes bool
	timing            *importTiming
}
type ImportOption func(*importConfig) error

const (
	defaultImportBatchSize          = 100000
	maxPreallocatedBatchSize        = 10000
	maxPacketImportBatchBytes       = 64 << 20
	deferQueryIndexesMinSourceBytes = 64 << 20
)

func WithContext(ctx context.Context) ImportOption {
	return func(c *importConfig) error {
		if ctx == nil {
			return fmt.Errorf("pcapdb: nil context")
		}
		c.ctx = ctx
		return nil
	}
}
func WithBatchSize(size int) ImportOption {
	return func(c *importConfig) error {
		if size < 1 || size > 100000 {
			return fmt.Errorf("pcapdb: batch size must be 1..100000")
		}
		c.batchSize = size
		return nil
	}
}

// WithProtocols enables BIN Parser messages and full structured JSONB fields.
func WithProtocols(enabled bool) ImportOption {
	return func(c *importConfig) error { c.protocols = enabled; return nil }
}

// WithStreams controls session indexing and bounded TCP/datagram stream BLOBs.
// Enabled by default. Disable it for a packet-only import with no replay cost.
func WithStreams(enabled bool) ImportOption {
	return func(c *importConfig) error { c.streams = enabled; return nil }
}

// WithTLSKeyLog enables authorized NSS key-log decryption during protocol
// replay. The key material is retained only for this import operation; its
// SHA256 is persisted so a different key log can invalidate cached analysis
// without storing the secrets themselves.
func WithTLSKeyLog(text string) ImportOption {
	return func(c *importConfig) error {
		keys, err := pcaputil.ParseTLSKeyLog(text)
		if err != nil {
			return fmt.Errorf("pcapdb: invalid TLS key log: %w", err)
		}
		digest := sha256.Sum256([]byte(text))
		c.tlsSecrets = keys
		c.tlsKeyLogSHA256 = hex.EncodeToString(digest[:])
		return nil
	}
}

// WithFieldIndex creates a scalar expression index for one frequently searched
// Fields JSON path. Requires WithProtocols(true). Repeated calls add paths; an
// already imported dataset can add indexes without replaying its capture.
func WithFieldIndex(path string) ImportOption {
	return func(c *importConfig) error {
		if err := validateProtocolFieldPath(path); err != nil {
			return err
		}
		for _, existing := range c.fieldIndexes {
			if existing == path {
				return nil
			}
		}
		if len(c.fieldIndexes) >= maxProtocolFieldIndexes {
			return fmt.Errorf("pcapdb: at most %d protocol field indexes are allowed", maxProtocolFieldIndexes)
		}
		c.fieldIndexes = append(c.fieldIndexes, path)
		return nil
	}
}

// The callback runs synchronously. It may inspect/list the catalog, but must not
// start another import of the same capture or close this manager.
func WithProgress(callback func(Progress)) ImportOption {
	return func(c *importConfig) error { c.progress = callback; return nil }
}

func (c *importConfig) notify(meta *PCAPFileDBMetadata) {
	if c.progress != nil {
		defer func() {
			if value := recover(); value != nil {
				if c.ctx.Err() != nil || (c.parent != nil && c.parent.Err() != nil) {
					c.progress = nil
					c.cancel()
					return
				}
				c.progressErr = fmt.Errorf("pcapdb: progress callback panicked: %v", value)
				c.progress = nil
				c.cancel()
			}
		}()
		c.progress(Progress{DatasetID: meta.DatasetID, State: meta.State, BytesIndexed: meta.BytesIndexed, TotalBytes: meta.SourceSize, PacketCount: meta.PacketCount, ProtocolCount: meta.ProtocolCount, SessionCount: meta.SessionCount, StreamCount: meta.StreamCount, StreamChunkCount: meta.StreamChunkCount})
	}
}

func (m *InstanceManager) GetOrCreate(filename string, options ...ImportOption) (_ *Database, resultErr error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	config := &importConfig{ctx: context.Background(), batchSize: defaultImportBatchSize, streams: true}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("pcapdb: nil import option")
		}
		if err := option(config); err != nil {
			return nil, err
		}
	}
	if len(config.fieldIndexes) != 0 && !config.protocols {
		return nil, fmt.Errorf("pcapdb: field indexes require withProtocols(true)")
	}
	if config.tlsSecrets != nil && !config.protocols {
		return nil, fmt.Errorf("pcapdb: TLS key log requires withProtocols(true)")
	}
	config.timing = newImportTiming()
	defer config.timing.report()
	setupStart := config.timing.begin()
	ctx, cancel := m.operationContext(config.ctx)
	defer cancel()
	config.ctx, config.cancel = ctx, cancel
	defer func() {
		if config.progressErr != nil && !errors.Is(resultErr, config.progressErr) {
			resultErr = errors.Join(resultErr, config.progressErr)
		}
	}()
	if config.parent != nil {
		stop := context.AfterFunc(config.parent, cancel)
		defer stop()
		if config.parent.Err() != nil {
			cancel()
		}
	}
	path, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	fingerprint, err := fingerprintFile(config.ctx, source)
	if err != nil {
		return nil, err
	}
	// Same fingerprints serialize before any provisional registration. The
	// lock is a scheduling hint, not a claim of equal content.
	lock, err := acquireFileLock(config.ctx, filepath.Join(m.root, ".locks", fingerprint.Hash+".lock"), true)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lock)
	var candidates []PCAPFileDBMetadata
	if err = m.profile.Where("fingerprint = ? AND fingerprint_version = ?", fingerprint.Hash, fingerprint.Version).Find(&candidates).Error; err != nil {
		return nil, err
	}
	fullHash := ""
	if fingerprint.Mode == "full-sha256" {
		fullHash = fingerprint.Hash
	} else if len(candidates) > 0 {
		fullHash, err = hashFile(config.ctx, source, info.Size())
		if err != nil {
			return nil, err
		}
	}
	if fullHash != "" {
		after, err := source.Stat()
		if err != nil {
			return nil, err
		}
		if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return nil, fmt.Errorf("pcapdb: source changed during fingerprinting")
		}
		if existing, err := m.findContent(fullHash); err != nil {
			return nil, err
		} else if existing != nil {
			return m.useExisting(source, info, fingerprint, path, existing, config)
		}
	}
	// A failed copy with no confirmed hash can reuse its registration; the
	// complete content is copied and verified again before publication.
	for _, candidate := range candidates {
		if candidate.FullSHA256 == "" && candidate.SourcePath == path {
			return m.useExisting(source, info, fingerprint, path, &candidate, config)
		}
	}
	id := uuid.NewString()
	directory := filepath.Join(m.root, id)
	if err = makeDurableDirectory(directory); err != nil {
		return nil, err
	}
	meta := &PCAPFileDBMetadata{
		Model:              gorm.Model{CreatedAt: time.Now().UTC()},
		DatasetID:          id,
		DatabaseType:       "sqlite",
		DatabasePath:       filepath.Join(directory, "index.sqlite"),
		SourcePath:         path,
		SourceAliases:      "[]",
		SourceSize:         info.Size(),
		SourceModTimeNS:    info.ModTime().UnixNano(),
		Fingerprint:        fingerprint.Hash,
		FingerprintMode:    fingerprint.Mode,
		FingerprintVersion: fingerprint.Version,
		SchemaVersion:      SchemaVersion,
		State:              StateRegistered,
	}

	datasetLock, err := acquireFileLock(config.ctx, datasetLockPath(meta), true)
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	defer releaseFileLock(datasetLock)
	if err = m.saveCatalog(meta); err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	config.timing.add(&config.timing.setup, setupStart)
	return m.buildDatasetLocked(source, info, fingerprint, path, meta, config)
}

func (m *InstanceManager) findContent(digest string) (*PCAPFileDBMetadata, error) {
	var meta PCAPFileDBMetadata
	err := m.profile.Where("full_sha256 = ?", digest).First(&meta).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	return &meta, err
}

func (m *InstanceManager) useExisting(source *os.File, info os.FileInfo, fp *Fingerprint, path string, meta *PCAPFileDBMetadata, config *importConfig) (*Database, error) {
	if err := makeDurableDirectory(filepath.Dir(meta.DatabasePath)); err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(config.ctx, datasetLockPath(meta), true)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lock)
	result, err := m.validateLocked(config.ctx, meta, false)
	if err != nil {
		return nil, err
	}
	meta = &result.Metadata
	if result.Valid {
		if err = m.addAlias(config.ctx, meta, path); err != nil {
			return nil, err
		}
		keyLogChanged := config.tlsSecrets != nil && config.tlsKeyLogSHA256 != meta.TLSKeyLogSHA256
		if (config.protocols && (!meta.ProtocolsIndexed || keyLogChanged)) || (config.streams && !meta.StreamsIndexed) {
			config.protocols = config.protocols || meta.ProtocolsIndexed
			config.streams = config.streams || meta.StreamsIndexed
			writer, err := openIndex(meta.DatabasePath, true, false)
			if err != nil {
				return nil, err
			}
			defer writer.Close()
			if err = m.indexProtocols(config.ctx, writer, meta, config); err != nil {
				return nil, m.failImport(writer, meta, err, config)
			}
		} else if len(config.fieldIndexes) > 0 {
			writer, err := openIndex(meta.DatabasePath, true, false)
			if err != nil {
				return nil, err
			}
			defer writer.Close()
			// Index-only changes are atomic and do not invalidate Ready data.
			if err = ensureProtocolFieldIndexes(config.ctx, writer, config.fieldIndexes); err != nil {
				return nil, err
			}
		}
		if len(config.fieldIndexes) > 0 {
			if err = m.refreshReader(config.ctx, meta); err != nil {
				return nil, err
			}
		}
		config.notify(meta)
		return m.openReady(meta, config.leased)
	}
	// Retry starts a fresh packet index, never a TCP replay from an arbitrary
	// packet offset. Existing committed counts describe only the failed attempt.
	return m.buildDatasetLocked(source, info, fp, path, meta, config)
}

func (m *InstanceManager) buildDatasetLocked(source *os.File, info os.FileInfo, fp *Fingerprint, path string, meta *PCAPFileDBMetadata, config *importConfig) (_ *Database, resultErr error) {
	_, statErr := os.Stat(meta.DatabasePath)
	create := errors.Is(statErr, os.ErrNotExist)
	writer, err := openIndex(meta.DatabasePath, true, create)
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	if create {
		if err = createIndex(config.ctx, writer, meta); err != nil {
			return nil, err
		}
	} else if _, err = readManifest(config.ctx, writer, meta.DatasetID); err != nil {
		// A crash before the initial schema transaction committed can leave an
		// empty SQLite file. Only that empty version-0 case may be initialized.
		var version, tables int
		scoped := indexWithContext(config.ctx, writer)
		if scoped.Raw("PRAGMA user_version").Row().Scan(&version) != nil || scoped.Table("sqlite_schema").Where("type = ?", "table").Count(&tables).Error != nil || version != 0 || tables != 0 {
			return nil, err
		}
		if err = createIndex(config.ctx, writer, meta); err != nil {
			return nil, err
		}
	}
	// Persist the index/WAL directory entries for both initial creation and a
	// retry of a schema transaction interrupted before its first commit.
	if err = syncDirectory(filepath.Dir(meta.DatabasePath)); err != nil {
		return nil, err
	}
	if info.Size() >= deferQueryIndexesMinSourceBytes {
		if err = dropPCAPQueryIndexes(writer); err != nil {
			return nil, err
		}
		config.deferQueryIndexes = true
	}
	defer func() {
		if resultErr != nil && meta != nil {
			resultErr = m.failImport(writer, meta, resultErr, config)
		}
	}()
	if meta.State == StateRegistered {
		config.notify(meta)
	}
	if err = m.transition(config.ctx, writer, meta, StateImporting); err != nil {
		return nil, err
	}
	meta.SourceSize, meta.SourceModTimeNS = info.Size(), info.ModTime().UnixNano()
	config.notify(meta)
	if err = m.transition(config.ctx, writer, meta, StateIndexing); err != nil {
		return nil, err
	}
	config.notify(meta)
	digest, err := m.ingestCapture(config.ctx, writer, source, meta, config)
	if err != nil {
		return nil, err
	}
	after, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return nil, fmt.Errorf("pcapdb: source changed during import")
	}
	if fp.Mode == "full-sha256" && fp.Hash != digest {
		return nil, fmt.Errorf("pcapdb: source hash changed during import")
	}
	if fp.Mode == "sampled-sha256" {
		actual, err := fingerprintFile(config.ctx, source)
		if err != nil {
			return nil, err
		}
		if actual.Hash != fp.Hash {
			return nil, fmt.Errorf("pcapdb: source samples changed during import")
		}
	}
	if meta.FullSHA256 != "" && meta.FullSHA256 != digest {
		return nil, fmt.Errorf("pcapdb: source changed while retrying a dataset")
	}
	if existing, err := m.findContent(digest); err != nil {
		return nil, err
	} else if existing != nil && existing.DatasetID != meta.DatasetID {
		// Another source or manager already registered these exact bytes.
		// Delete only this provisional, package-owned dataset, never input files.
		if meta.FullSHA256 != "" || meta.DatabasePath != filepath.Join(m.root, meta.DatasetID, "index.sqlite") {
			return nil, fmt.Errorf("pcapdb: refusing to discard a non-provisional dataset")
		}
		writer.Close()
		removed := m.profile.Where("full_sha256 = ''").Delete(meta)
		if err = removed.Error; err != nil {
			return nil, err
		}
		if removed.RowsAffected != 1 {
			return nil, fmt.Errorf("pcapdb: provisional registration changed during deduplication")
		}
		if err = os.RemoveAll(filepath.Dir(meta.DatabasePath)); err != nil {
			return nil, err
		}
		meta = nil // the provisional dataset no longer has a failure checkpoint
		return m.useExisting(source, info, fp, path, existing, config)
	}
	meta.FullSHA256 = digest
	indexStart := config.timing.begin()
	if err = ensureProtocolFieldIndexes(config.ctx, writer, config.fieldIndexes); err != nil {
		return nil, err
	}
	if config.deferQueryIndexes {
		if err = ensurePCAPQueryIndexes(writer); err != nil {
			return nil, err
		}
	}
	config.timing.add(&config.timing.finalIndexes, indexStart)
	finalizeStart := config.timing.begin()
	if err = m.transition(config.ctx, writer, meta, StateReady); err != nil {
		return nil, err
	}
	if err = m.addAlias(config.ctx, meta, path); err != nil {
		return nil, err
	}
	config.notify(meta)
	if err = checkpointIndex(config.ctx, writer); err != nil {
		return nil, err
	}
	config.timing.add(&config.timing.finalize, finalizeStart)
	return m.openReady(meta, config.leased)
}

func (m *InstanceManager) failImport(writer *gorm.DB, meta *PCAPFileDBMetadata, cause error, config *importConfig) error {
	// Counters in the working struct may include rows rolled back by the
	// current batch. Only the committed manifest is a recovery checkpoint.
	if committed, err := readManifest(context.Background(), writer, meta.DatasetID); err == nil {
		*meta = *committed
		if meta.State == StateReady {
			return cause
		}
	}
	meta.State = StateFailed
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		meta.State = StateInterrupted
	}
	if config.progressErr != nil {
		meta.State = StateFailed
		cause = config.progressErr
	}
	meta.LastError = cause.Error()
	// A canceled operation still publishes its final durable failure state.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := m.checkpoint(ctx, writer, meta)
	config.notify(meta)
	return errors.Join(cause, err)
}

func newOfflinePacketAnalyzer(ctx context.Context, options []pcaputil.CaptureOption) (*pcaputil.PacketAnalyzer, context.CancelFunc, error) {
	analysisCtx, cancel := context.WithCancel(ctx)
	options = append(options, pcaputil.WithContext(analysisCtx))
	analyzer, err := pcaputil.NewPacketAnalyzer(options...)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return analyzer, cancel, nil
}

// ingestCapture reads the source once. Physical packets and protocol analysis
// consume the same bounded record, and their pending rows share one transaction.
func (m *InstanceManager) ingestCapture(ctx context.Context, db *gorm.DB, file *os.File, meta *PCAPFileDBMetadata, config *importConfig) (_ string, resultErr error) {
	ingestStart := config.timing.begin()
	defer config.timing.add(&config.timing.ingest, ingestStart)
	reader, err := newPacketReader(file)
	if err != nil {
		return "", err
	}
	meta.Format = reader.format
	resetStart := config.timing.begin()
	reset := db.BeginTx(ctx, nil)
	if reset.Error != nil {
		return "", reset.Error
	}
	defer reset.Rollback()
	scoped := indexWithContext(ctx, reset)
	for _, model := range []interface{}{&PCAPMessagePacket{}, &PCAPProtocolMessage{}, &PCAPStreamChunkPacket{}, &PCAPSessionPacket{}, &PCAPStreamChunk{}, &PCAPStream{}, &PCAPSession{}, &PCAPCaptureRecord{}, &Packet{}, &PCAPCaptureInterface{}} {
		if err := scoped.Unscoped().Where("1 = 1").Delete(model).Error; err != nil {
			return "", err
		}
	}
	// Rows, BLOBs and counters reset atomically. No external byte store can
	// outrun the SQLite checkpoint if a batch is killed before COMMIT.
	meta.BytesIndexed, meta.PacketCount, meta.CaptureRecordCount = 0, 0, 0
	meta.ProtocolCount, meta.ProtocolDataSize, meta.ProtocolsIndexed = 0, 0, false
	meta.ProtocolDataSHA256, meta.StreamDataSHA256 = "", ""
	meta.SessionCount, meta.StreamCount, meta.StreamChunkCount, meta.StreamDataSize, meta.StreamsIndexed = 0, 0, 0, 0, false
	if err := writeManifest(ctx, scoped, meta); err != nil {
		return "", err
	}
	if err := reset.Commit().Error; err != nil {
		return "", err
	}
	config.timing.add(&config.timing.setup, resetStart)

	packets := make([]Packet, 0, min(config.batchSize, maxPreallocatedBatchSize))
	records := make([]PCAPCaptureRecord, 0, min(config.batchSize, maxPreallocatedBatchSize))
	interfaces := make([]PCAPCaptureInterface, 0)
	interfaceIDs := make(map[[2]int64]int64)
	hasher := sha256.New()
	analysis := newProtocolBatch(ctx, db, meta, config)
	var analyzer *pcaputil.PacketAnalyzer
	var cancelAnalysis context.CancelFunc
	analysisClosed := false
	defer func() {
		if analyzer != nil && !analysisClosed {
			analysis.mu.Lock()
			if analysis.err == nil {
				if resultErr != nil {
					analysis.err = resultErr
				} else {
					analysis.err = fmt.Errorf("pcapdb: capture ingestion stopped before analyzer close")
				}
			}
			analysis.mu.Unlock()
			resultErr = errors.Join(resultErr, analyzer.Close())
			analysisClosed = true
		}
		if cancelAnalysis != nil {
			cancelAnalysis()
		}
	}()
	var lastCatalog time.Time
	var batchBytes int
	// flushLocked requires analysis.mu.
	flushLocked := func(final bool) error {
		if config.timing.enabled {
			config.timing.flushCount.Add(1)
		}
		flushStart := config.timing.begin()
		defer config.timing.add(&config.timing.dbFlush, flushStart)
		tx := db.BeginTx(ctx, nil)
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		scoped := indexWithContext(ctx, tx)
		packetInsertStart := config.timing.begin()
		if err := scoped.CreateInBatches(&interfaces).Error; err != nil {
			return err
		}
		config.timing.add(&config.timing.interfaceInsert, packetInsertStart)
		packetRowStart := config.timing.begin()
		if err := scoped.CreateInBatches(&packets).Error; err != nil {
			return err
		}
		config.timing.add(&config.timing.packetRowInsert, packetRowStart)
		captureRecordStart := config.timing.begin()
		if err := scoped.CreateInBatches(&records).Error; err != nil {
			return err
		}
		config.timing.add(&config.timing.captureRecordInsert, captureRecordStart)
		config.timing.add(&config.timing.packetInsert, packetInsertStart)
		if err := analysis.flush(ctx, scoped); err != nil {
			return err
		}
		if final {
			meta.ProtocolsIndexed, meta.StreamsIndexed = config.protocols, config.streams
			if config.protocols {
				meta.ProtocolDataSHA256 = hex.EncodeToString(analysis.hasher.Sum(nil))
			} else {
				meta.ProtocolDataSHA256 = ""
			}
			if config.streams {
				meta.StreamDataSHA256 = analysis.sessions.digest()
			} else {
				meta.StreamDataSHA256 = ""
			}
		}
		meta.UpdatedAt = time.Now().UTC()
		manifestStart := config.timing.begin()
		if err := writeManifest(ctx, scoped, meta); err != nil {
			return err
		}
		config.timing.add(&config.timing.manifestWrite, manifestStart)
		commitStart := config.timing.begin()
		if err := tx.Commit().Error; err != nil {
			return err
		}
		config.timing.add(&config.timing.transaction, commitStart)
		if final || time.Since(lastCatalog) >= 250*time.Millisecond {
			catalogStart := config.timing.begin()
			if err := m.saveCatalog(meta); err != nil {
				return err
			}
			config.timing.add(&config.timing.catalog, catalogStart)
			lastCatalog = time.Now()
		}
		config.notify(meta)
		clear(packets)
		packets = packets[:0]
		clear(records)
		records = records[:0]
		interfaces = interfaces[:0]
		analysis.clear()
		batchBytes = 0
		return ctx.Err()
	}
	readSpan := func(start, end int64) ([]byte, error) {
		if start < 0 || end < start || end > meta.SourceSize || end-start > maxCaptureBlock {
			return nil, fmt.Errorf("pcapdb: invalid capture record")
		}
		data := make([]byte, int(end-start))
		if len(data) > 0 {
			_, err := file.ReadAt(data, start)
			if err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	storeGap := func(end int64) error {
		if end < meta.BytesIndexed || end > meta.SourceSize {
			return fmt.Errorf("pcapdb: invalid capture framing order")
		}
		for meta.BytesIndexed < end {
			next := min(end, meta.BytesIndexed+captureMetadataChunkBytes)
			data, err := readSpan(meta.BytesIndexed, next)
			if err != nil {
				return err
			}
			meta.CaptureRecordCount++
			records = append(records, PCAPCaptureRecord{Model: gorm.Model{ID: uint(meta.CaptureRecordCount)}, Prefix: data, Suffix: []byte{}})
			hasher.Write(data)
			batchBytes += len(data)
			meta.BytesIndexed = next
			if batchBytes >= maxPacketImportBatchBytes || len(records) >= max(2000, config.batchSize) {
				if err := flushLocked(false); err != nil {
					return err
				}
			}
		}
		return nil
	}

	consume := func(work func() error) {
		analysis.mu.Lock()
		defer analysis.mu.Unlock()
		if analysis.err == nil {
			analysis.err = work()
		}
	}
	if config.protocols || config.streams {
		if config.protocols {
			if err = ensureProtocolCompletionColumn(ctx, db); err != nil {
				return "", err
			}
		}
		options := []pcaputil.CaptureOption{
			pcaputil.WithCaptureReplayClock(),
			pcaputil.WithHTTPResponseWithoutRequest(true),
			pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true, Stream: true, Workers: 1, MaxFrameBytes: streamChunkBytes, MaxFlows: maxActiveSessions, MaxPendingBytes: 2 << 20, MaxTotalPendingBytes: 16 << 20, MaxPendingSegments: 4096, MaxTotalPendingSegments: 32768}),
		}
		if config.protocols {
			options = append(options, pcaputil.WithOnProtocolMessage(func(event *pcaputil.ProtocolEvent) {
				callbackStart := config.timing.begin()
				consume(func() error {
					assembleStart := config.timing.begin()
					err := analysis.appendEvent(event)
					config.timing.add(&config.timing.protocolAssemble, assembleStart)
					if err != nil {
						return err
					}
					if analysis.full() {
						return flushLocked(false)
					}
					return nil
				})
				config.timing.add(&config.timing.protocolCallback, callbackStart)
			}))
		}
		if config.streams {
			options = append(options,
				pcaputil.WithBeforeTransportPacket(func(packet gopacket.Packet) { consume(func() error { return analysis.sessions.datagram(packet) }) }),
				pcaputil.WithOnTrafficFlowPacket(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time) {
					consume(func() error { return analysis.sessions.tcpPacket(flow, conn, refs, ts) })
				}),
				pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
					consume(func() error { return analysis.sessions.tcpData(flow, conn, frame) })
				}),
				pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
					consume(func() error { analysis.sessions.closeTCP(reason, flow); return nil })
				}),
			)
		}
		if config.tlsSecrets != nil {
			options = append(options, pcaputil.WithTLSSecrets(config.tlsSecrets))
		}
		analyzer, cancelAnalysis, err = newOfflinePacketAnalyzer(ctx, options)
		if err != nil {
			return "", err
		}
	}
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		readStart := config.timing.begin()
		raw, ci, location, readErr := reader.next()
		config.timing.add(&config.timing.captureRead, readStart)
		analysis.mu.Lock()
		for _, iface := range reader.takeInterfaces() {
			key := [2]int64{iface.section, int64(iface.id)}
			id := int64(len(interfaceIDs) + 1)
			interfaceIDs[key] = id
			interfaces = append(interfaces, PCAPCaptureInterface{Model: gorm.Model{ID: uint(id)}, Section: iface.section, Interface: iface.id, LinkType: int(iface.link), Snaplen: iface.snaplen, TimestampResolution: iface.resolution, TimestampOffset: iface.offset, Name: iface.name})
		}
		if readErr == io.EOF {
			if err = storeGap(meta.SourceSize); err != nil {
				analysis.mu.Unlock()
				return "", err
			}
			analysis.mu.Unlock()
			break
		}
		if readErr != nil {
			analysis.mu.Unlock()
			return "", readErr
		}
		if err = storeGap(location.recordOffset); err != nil {
			analysis.mu.Unlock()
			return "", err
		}
		prefix, err := readSpan(location.recordOffset, location.dataOffset)
		if err != nil {
			analysis.mu.Unlock()
			return "", err
		}
		suffix, err := readSpan(location.dataOffset+int64(len(raw)), location.endOffset)
		if err != nil {
			analysis.mu.Unlock()
			return "", err
		}
		// The PCAPNG reader reuses its packet buffer. Own the bytes before the
		// next read, including a zero-length packet's non-NULL empty BLOB.
		data := append(make([]byte, 0, len(raw)), raw...)
		metadataStart := config.timing.begin()
		packet := packetMetadata(data, reader.link)
		config.timing.add(&config.timing.packetMetadata, metadataStart)
		meta.PacketCount++
		packet.ID = uint(meta.PacketCount)
		packet.CaptureInterfaceID = interfaceIDs[[2]int64{location.section, int64(location.iface)}]
		if packet.CaptureInterfaceID == 0 {
			analysis.mu.Unlock()
			return "", fmt.Errorf("pcapdb: packet has unknown interface")
		}
		packet.Section, packet.Interface, packet.LinkType = location.section, location.iface, int(reader.link)
		packet.CapturedLength, packet.OriginalLength, packet.Data = ci.CaptureLength, ci.Length, data
		if location.hasTimestamp {
			packet.TimestampNS = timestampNano(ci.Timestamp)
		}
		packets = append(packets, packet)
		meta.CaptureRecordCount++
		id := packet.ID
		records = append(records, PCAPCaptureRecord{Model: gorm.Model{ID: uint(meta.CaptureRecordCount)}, PacketID: &id, Prefix: prefix, Suffix: suffix})
		hasher.Write(prefix)
		hasher.Write(data)
		hasher.Write(suffix)
		batchBytes += len(prefix) + len(data) + len(suffix)
		meta.BytesIndexed = location.endOffset
		analysis.mu.Unlock()
		if analyzer != nil {
			ref := pcaputil.PacketReference{
				Number: uint64(packet.ID),
				Domain: pcaputil.CaptureDomain{Section: uint32(location.section), Interface: location.iface},
			}
			feedStart := config.timing.begin()
			err = analyzer.FeedWithReference(data, ci, reader.link, ref)
			config.timing.add(&config.timing.analyzerFeed, feedStart)
			if err != nil {
				analysis.mu.Lock()
				if analysis.err == nil {
					analysis.err = err
				}
				analysis.mu.Unlock()
				return "", err
			}
		}
		analysis.mu.Lock()
		if analysis.err == nil && (len(packets) >= config.batchSize || batchBytes >= maxPacketImportBatchBytes || analysis.full()) {
			analysis.err = flushLocked(false)
		}
		err = analysis.err
		analysis.mu.Unlock()
		if err != nil {
			return "", err
		}
	}
	if analyzer != nil {
		closeStart := config.timing.begin()
		closeErr := analyzer.Close()
		config.timing.add(&config.timing.analysisClose, closeStart)
		analysisClosed = true
		if closeErr != nil {
			return "", closeErr
		}
	}
	analysis.mu.Lock()
	defer analysis.mu.Unlock()
	if analysis.err != nil {
		return "", analysis.err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	analysis.sessions.finish()
	if err = flushLocked(true); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func timestampNano(timestamp time.Time) *int64 {
	seconds := timestamp.Unix()
	if seconds > math.MaxInt64/int64(time.Second) || seconds < math.MinInt64/int64(time.Second) {
		return nil
	}
	value := seconds * int64(time.Second)
	if value > math.MaxInt64-int64(timestamp.Nanosecond()) {
		return nil
	}
	result := value + int64(timestamp.Nanosecond())
	return &result
}

func packetMetadata(raw []byte, link layers.LinkType) Packet {
	result := Packet{}
	packet := gopacket.NewPacket(raw, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	// Store the outer Ethernet observation. Destination MAC is still retained
	// for packet evidence, but endpoint summaries use only source MAC/IP pairs.
	if ethernet, ok := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet); ok {
		result.SourceMAC = ethernet.SrcMAC.String()
		result.DestinationMAC = ethernet.DstMAC.String()
	}
	var vlans []string
	for _, layer := range packet.Layers() {
		if vlan, ok := layer.(*layers.Dot1Q); ok {
			vlans = append(vlans, strconv.Itoa(int(vlan.VLANIdentifier)))
		}
	}
	result.VLANStack = strings.Join(vlans, ",")
	if network := packet.NetworkLayer(); network != nil {
		result.SourceIP, result.DestinationIP = network.NetworkFlow().Src().String(), network.NetworkFlow().Dst().String()
	}
	switch transport := packet.TransportLayer().(type) {
	case *layers.TCP:
		result.Transport = "tcp"
		result.SourcePort, result.DestinationPort = int(transport.SrcPort), int(transport.DstPort)
	case *layers.UDP:
		result.Transport = "udp"
		result.SourcePort, result.DestinationPort = int(transport.SrcPort), int(transport.DstPort)
	case *layers.SCTP:
		result.Transport = "sctp"
		result.SourcePort, result.DestinationPort = int(transport.SrcPort), int(transport.DstPort)
	default:
		if ip, ok := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4); ok {
			result.Transport = strings.ToLower(ip.Protocol.String())
		}
		if ip, ok := packet.Layer(layers.LayerTypeIPv6).(*layers.IPv6); ok {
			result.Transport = strings.ToLower(ip.NextHeader.String())
		}
		if packet.Layer(layers.LayerTypeARP) != nil {
			result.Transport = "arp"
		}
	}
	if failed := packet.ErrorLayer(); failed != nil {
		result.DecodeError = failed.Error().Error()
	}
	result.Summary = packetFrameSummary(packet, result)
	return result
}
