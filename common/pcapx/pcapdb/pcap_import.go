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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/gorm"
)

type importConfig struct {
	ctx          context.Context
	parent       context.Context
	batchSize    int
	protocols    bool
	streams      bool
	fieldIndexes []string
	progress     func(Progress)
	cancel       context.CancelFunc
	progressErr  error
}
type ImportOption func(*importConfig) error

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
	config := &importConfig{ctx: context.Background(), batchSize: 2000, streams: true}
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
		if (config.protocols && !meta.ProtocolsIndexed) || (config.streams && !meta.StreamsIndexed) {
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
		return m.openReady(meta)
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
	digest, err := m.indexPackets(config.ctx, writer, source, meta, config)
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
	if config.protocols || config.streams {
		if err = m.indexProtocols(config.ctx, writer, meta, config); err != nil {
			return nil, err
		}
	} else if err = m.transition(config.ctx, writer, meta, StateReady); err != nil {
		return nil, err
	}
	if err = m.addAlias(config.ctx, meta, path); err != nil {
		return nil, err
	}
	config.notify(meta)
	if err = checkpointIndex(config.ctx, writer); err != nil {
		return nil, err
	}
	return m.openReady(meta)
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

func (m *InstanceManager) indexPackets(ctx context.Context, db *gorm.DB, file *os.File, meta *PCAPFileDBMetadata, config *importConfig) (string, error) {
	reader, err := newPacketReader(file)
	if err != nil {
		return "", err
	}
	meta.Format = reader.format
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

	packets := make([]Packet, 0, min(config.batchSize, 2000))
	records := make([]PCAPCaptureRecord, 0)
	interfaces := make([]PCAPCaptureInterface, 0)
	interfaceIDs := make(map[[2]int64]int64)
	hasher := sha256.New()
	var lastCatalog time.Time
	var batchBytes int
	commit := func(final bool) error {
		tx := db.BeginTx(ctx, nil)
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		scoped := indexWithContext(ctx, tx)
		for _, rows := range []interface{}{&interfaces, &packets, &records} {
			if err := scoped.CreateInBatches(rows).Error; err != nil {
				return err
			}
		}
		meta.UpdatedAt = time.Now().UTC()
		if err := writeManifest(ctx, scoped, meta); err != nil {
			return err
		}
		if err := tx.Commit().Error; err != nil {
			return err
		}
		if final || time.Since(lastCatalog) >= 250*time.Millisecond {
			if err := m.saveCatalog(meta); err != nil {
				return err
			}
			config.notify(meta)
			lastCatalog = time.Now()
		}
		clear(packets)
		packets = packets[:0]
		clear(records)
		records = records[:0]
		interfaces = interfaces[:0]
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
			if batchBytes >= maxImportBatchBytes || len(records) >= max(2000, config.batchSize) {
				if err := commit(false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		raw, ci, location, readErr := reader.next()
		for _, iface := range reader.takeInterfaces() {
			key := [2]int64{iface.section, int64(iface.id)}
			id := int64(len(interfaceIDs) + 1)
			interfaceIDs[key] = id
			interfaces = append(interfaces, PCAPCaptureInterface{Model: gorm.Model{ID: uint(id)}, Section: iface.section, Interface: iface.id, LinkType: int(iface.link), Snaplen: iface.snaplen, TimestampResolution: iface.resolution, TimestampOffset: iface.offset, Name: iface.name})
		}
		if readErr == io.EOF {
			if err = storeGap(meta.SourceSize); err != nil {
				return "", err
			}
			if err = commit(true); err != nil {
				return "", err
			}
			return hex.EncodeToString(hasher.Sum(nil)), nil
		}
		if readErr != nil {
			return "", readErr
		}
		if err = storeGap(location.recordOffset); err != nil {
			return "", err
		}
		prefix, err := readSpan(location.recordOffset, location.dataOffset)
		if err != nil {
			return "", err
		}
		suffix, err := readSpan(location.dataOffset+int64(len(raw)), location.endOffset)
		if err != nil {
			return "", err
		}
		// The PCAPNG reader reuses its packet buffer. Own the bytes before the
		// next read, including a zero-length packet's non-NULL empty BLOB.
		data := append(make([]byte, 0, len(raw)), raw...)
		packet := packetMetadata(data, reader.link)
		meta.PacketCount++
		packet.ID = uint(meta.PacketCount)
		packet.CaptureInterfaceID = interfaceIDs[[2]int64{location.section, int64(location.iface)}]
		if packet.CaptureInterfaceID == 0 {
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
		if len(packets) >= config.batchSize || batchBytes >= maxImportBatchBytes {
			if err = commit(false); err != nil {
				return "", err
			}
		}
	}
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
	return result
}
