package pcapdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
)

// LiveCaptureConfig describes one continuous, single-interface capture. A zero
// Duration runs until its owning context is cancelled. FlushInterval controls
// visibility latency, never capture duration or retention.
type LiveCaptureConfig struct {
	Interface     string
	BPF           string
	Snaplen       int
	Promisc       bool
	Duration      time.Duration
	FlushInterval time.Duration
	BatchSize     int
}

func (c LiveCaptureConfig) normalized() (LiveCaptureConfig, error) {
	if c.Snaplen == 0 {
		c.Snaplen = 65535
	}
	if c.BatchSize == 0 {
		c.BatchSize = 2000
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 250 * time.Millisecond
	}
	if c.Snaplen < 1 || c.Snaplen > maxCaptureBlock || c.BatchSize < 1 || c.BatchSize > 100000 || c.FlushInterval < time.Millisecond || c.FlushInterval > time.Minute || c.Duration < 0 {
		return c, fmt.Errorf("pcapdb: invalid live capture limits")
	}
	return c, nil
}

// LiveWriter serializes packet ingress and keeps the same analyzer alive across
// commits. It owns the writer connection; query handles own independent readers.
// Close joins analyzer cleanup and the final transaction before releasing locks.
type LiveWriter struct {
	ID          string
	inputMu     sync.Mutex
	batch       *protocolBatch
	manager     *InstanceManager
	writer      *gorm.DB
	lock        *os.File
	ctx         context.Context
	cancel      context.CancelFunc
	analyzer    *pcaputil.PacketAnalyzer
	link        layers.LinkType
	config      LiveCaptureConfig
	buffer      bytes.Buffer
	encoder     *pcapgo.Writer
	hasher      hash.Hash
	packets     []Packet
	records     []PCAPCaptureRecord
	packetBytes int
	committed   PCAPFileDBMetadata
	closed      bool
	closeErr    error
	done        chan struct{}
}

// CreateLiveDataset also accepts a controlled packet source through Feed. This
// makes the complete storage/query contract usable without native pcap drivers.
func (m *InstanceManager) CreateLiveDataset(ctx context.Context, config LiveCaptureConfig, link layers.LinkType) (*LiveWriter, error) {
	if ctx == nil {
		return nil, fmt.Errorf("pcapdb: live capture context is required")
	}
	config, err := config.normalized()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = m.begin(); err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			m.ops.Done()
		}
	}()
	operation, cancel := m.operationContext(ctx)
	defer func() {
		if !owned {
			cancel()
		}
	}()
	id := uuid.NewString()
	dir := filepath.Join(m.root, id)
	if err = makeDurableDirectory(dir); err != nil {
		return nil, err
	}
	meta := &PCAPFileDBMetadata{DatasetID: id, DatabaseType: "sqlite", DatabasePath: filepath.Join(dir, "index.sqlite"), FingerprintMode: liveFingerprintMode, SourceAliases: "[]", Format: "pcap", SchemaVersion: SchemaVersion, State: StateRegistered, ProtocolsIndexed: true, StreamsIndexed: true}
	lock, err := acquireFileLock(operation, datasetLockPath(meta), true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !owned {
			releaseFileLock(lock)
		}
	}()
	db, err := openIndex(meta.DatabasePath, true, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !owned {
			db.Close()
		}
	}()
	if err = createIndex(operation, db, meta); err != nil {
		return nil, err
	}
	w := &LiveWriter{ID: id, manager: m, writer: db, lock: lock, ctx: operation, cancel: cancel, link: link, config: config, hasher: sha256.New(), done: make(chan struct{})}
	w.encoder = pcapgo.NewWriterNanos(&w.buffer)
	if err = w.encoder.WriteFileHeader(uint32(config.Snaplen), link); err != nil {
		return nil, err
	}
	header := append([]byte{}, w.buffer.Bytes()...)
	w.buffer.Reset()
	w.hasher.Write(header)
	meta.SourceSize = int64(len(header))
	meta.BytesIndexed = meta.SourceSize
	meta.CaptureRecordCount = 1
	w.records = append(w.records, PCAPCaptureRecord{Model: gorm.Model{ID: 1}, Prefix: header, Suffix: []byte{}})
	w.batch = newProtocolBatch(context.Background(), db, meta, &importConfig{batchSize: config.BatchSize, protocols: true, streams: true})
	// A single analysis worker plus the batch mutex also serializes idle expiry
	// callbacks. Feed never holds the batch mutex while entering the analyzer.
	consume := func(fn func() error) {
		w.batch.mu.Lock()
		defer w.batch.mu.Unlock()
		if w.batch.err == nil {
			w.batch.err = fn()
		}
	}
	w.analyzer, err = pcaputil.NewPacketAnalyzer(
		pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{Stream: true, Workers: 1, MaxFrameBytes: streamChunkBytes, MaxFlows: maxActiveSessions, MaxPendingBytes: 2 << 20, MaxTotalPendingBytes: 16 << 20, MaxPendingSegments: 4096, MaxTotalPendingSegments: 32768}),
		pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { consume(func() error { return w.batch.appendEvent(e) }) }),
		pcaputil.WithBeforeTransportPacket(func(p gopacket.Packet) { consume(func() error { return w.batch.sessions.datagram(p) }) }),
		pcaputil.WithOnTrafficFlowPacket(func(f *pcaputil.TrafficFlow, c *pcaputil.TrafficConnection, r []pcaputil.PacketReference, ts time.Time) {
			consume(func() error { return w.batch.sessions.tcpPacket(f, c, r, ts) })
		}),
		pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(f *pcaputil.TrafficFlow, c *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			consume(func() error { return w.batch.sessions.tcpData(f, c, frame) })
		}),
		pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, f *pcaputil.TrafficFlow) {
			consume(func() error { w.batch.sessions.closeTCP(reason, f); return nil })
		}),
	)
	if err != nil {
		return nil, err
	}
	meta.State = StateReady
	if err = w.flushLocked(); err != nil {
		w.analyzer.Close()
		return nil, err
	}
	if err = syncDirectory(dir); err != nil {
		w.analyzer.Close()
		return nil, err
	}
	owned = true
	go func() {
		ticker := time.NewTicker(config.FlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-w.done:
				return
			case <-operation.Done():
				_ = w.Close()
				return
			case <-ticker.C:
				if err := w.Flush(); err != nil {
					_ = w.Close()
					return
				}
			}
		}
	}()
	return w, nil
}

func (w *LiveWriter) Feed(raw []byte, ci gopacket.CaptureInfo) error {
	w.inputMu.Lock()
	defer w.inputMu.Unlock()
	if w.closed {
		return ErrClosed
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if ci.CaptureLength != len(raw) || ci.Length < len(raw) || len(raw) > w.config.Snaplen {
		return fmt.Errorf("pcapdb: invalid live packet lengths")
	}
	// Input is one physical interface. Do not inherit unrelated evidence IDs
	// from a replay source; PacketAnalyzer assigns the same sequential numbers.
	ci.AncillaryData = nil
	ci.InterfaceIndex = 0
	w.batch.mu.Lock()
	if w.batch.err != nil {
		err := w.batch.err
		w.batch.mu.Unlock()
		return err
	}
	if err := w.encoder.WritePacket(ci, raw); err != nil {
		w.batch.mu.Unlock()
		return err
	}
	encoded := w.buffer.Bytes()
	w.batch.meta.PacketCount++
	id := uint(w.batch.meta.PacketCount)
	packet := packetMetadata(raw, w.link)
	packet.Model = gorm.Model{ID: id}
	packet.CaptureInterfaceID = 1
	packet.LinkType = int(w.link)
	packet.TimestampNS = timestampNano(ci.Timestamp)
	packet.Data = append([]byte{}, raw...)
	packet.CapturedLength = len(raw)
	packet.OriginalLength = ci.Length
	w.packets = append(w.packets, packet)
	w.batch.meta.CaptureRecordCount++
	w.records = append(w.records, PCAPCaptureRecord{Model: gorm.Model{ID: uint(w.batch.meta.CaptureRecordCount)}, PacketID: &id, Prefix: append([]byte{}, encoded[:16]...), Suffix: []byte{}})
	w.hasher.Write(encoded)
	w.batch.meta.SourceSize += int64(len(encoded))
	w.batch.meta.BytesIndexed = w.batch.meta.SourceSize
	w.packetBytes += len(encoded)
	w.buffer.Reset()
	w.batch.mu.Unlock()
	if err := w.analyzer.Feed(raw, ci, w.link); err != nil {
		w.batch.mu.Lock()
		w.batch.err = err
		w.batch.mu.Unlock()
		return err
	}
	w.batch.mu.Lock()
	defer w.batch.mu.Unlock()
	if w.batch.err != nil {
		return w.batch.err
	}
	if len(w.packets) >= w.config.BatchSize || w.packetBytes+w.batch.bytes+w.batch.sessions.bytes >= maxImportBatchBytes || w.batch.full() {
		return w.flushLocked()
	}
	return nil
}

func (w *LiveWriter) Flush() error {
	w.inputMu.Lock()
	defer w.inputMu.Unlock()
	if w.closed {
		return w.closeErr
	}
	w.batch.mu.Lock()
	defer w.batch.mu.Unlock()
	return w.flushLocked()
}

// flushLocked holds batch.mu (except initial construction before publication).
func (w *LiveWriter) flushLocked() error {
	b := w.batch
	if b.err != nil {
		return b.err
	}
	if len(w.records) == 0 && len(b.messages) == 0 && b.sessions.pending() == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx := w.writer.BeginTx(ctx, nil)
	if tx.Error != nil {
		b.err = tx.Error
		return b.err
	}
	defer tx.Rollback()
	db := indexWithContext(ctx, tx)
	persist := func() error {
		if w.committed.DatasetID == "" {
			iface := PCAPCaptureInterface{Model: gorm.Model{ID: 1}, LinkType: int(w.link), Snaplen: uint32(w.config.Snaplen), TimestampResolution: 9, Name: w.config.Interface}
			if err := db.Create(&iface).Error; err != nil {
				return err
			}
		}
		for _, rows := range []interface{}{&w.packets, &w.records} {
			if err := db.CreateInBatches(rows).Error; err != nil {
				return err
			}
		}
		if err := b.flush(ctx, db); err != nil {
			return err
		}
		b.meta.Fingerprint = hex.EncodeToString(w.hasher.Sum(nil))
		b.meta.ProtocolDataSHA256 = hex.EncodeToString(b.hasher.Sum(nil))
		b.meta.StreamDataSHA256 = b.sessions.digest()
		b.meta.UpdatedAt = time.Now().UTC()
		if err := writeManifest(ctx, db, b.meta); err != nil {
			return err
		}
		return tx.Commit().Error
	}
	if err := persist(); err != nil {
		b.err = err
		return err
	}
	w.committed = *b.meta
	clear(w.packets)
	w.packets = w.packets[:0]
	clear(w.records)
	w.records = w.records[:0]
	w.packetBytes = 0
	b.clear()
	if err := w.manager.saveCatalog(b.meta); err != nil {
		b.err = err
		return err
	}
	return nil
}

func (w *LiveWriter) Metadata() PCAPFileDBMetadata {
	w.batch.mu.Lock()
	defer w.batch.mu.Unlock()
	return w.committed
}

func (w *LiveWriter) Close() error { return w.finish(nil) }

func (w *LiveWriter) finish(cause error) error {
	w.inputMu.Lock()
	defer w.inputMu.Unlock()
	if w.closed {
		return w.closeErr
	}
	// Analyzer uses its own context so cancellation of ingress cannot discard
	// closing protocol events before the final database transaction.
	analysisErr := w.analyzer.Close()
	w.batch.mu.Lock()
	defer w.batch.mu.Unlock()
	w.closed = true
	w.batch.sessions.finish()
	if w.batch.err == nil {
		w.batch.err = errors.Join(cause, analysisErr)
	}
	if w.batch.err == nil {
		w.closeErr = w.flushLocked()
	} else {
		w.closeErr = w.batch.err
	}
	if w.closeErr != nil {
		// Drop only the uncommitted batch. Readers keep the last atomic commit.
		meta := w.committed
		meta.LastError = w.closeErr.Error()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		checkpointErr := w.manager.checkpoint(ctx, w.writer, &meta)
		cancel()
		if checkpointErr == nil {
			w.committed = meta
		}
		w.closeErr = errors.Join(w.closeErr, checkpointErr)
	}
	w.closeErr = errors.Join(w.closeErr, w.writer.Close())
	releaseFileLock(w.lock)
	w.cancel()
	close(w.done)
	w.manager.ops.Done()
	return w.closeErr
}

// RunLiveCapture blocks for the capture's lifetime. Async scheduling belongs to
// the caller. onStarted runs only after both the native handle and dataset exist.
func (m *InstanceManager) RunLiveCapture(ctx context.Context, config LiveCaptureConfig, onStarted func(PCAPFileDBMetadata)) (meta *PCAPFileDBMetadata, resultErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("pcapdb: capture context is required")
	}
	// Include the native handle in manager shutdown, not only the writer. The
	// handle's deferred Close executes before this operation releases its slot.
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	ctx, stop := m.operationContext(ctx)
	defer stop()
	config, err := config.normalized()
	if err != nil {
		return nil, err
	}
	if config.Interface == "" {
		return nil, fmt.Errorf("pcapdb: select one capture interface")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	device, err := pcaputil.IfaceNameToPcapIfaceName(config.Interface)
	if err != nil {
		return nil, err
	}
	h, err := openLiveWithPermission(ctx, func() (*pcap.Handle, error) {
		return pcap.OpenLive(device, int32(config.Snaplen), config.Promisc, 100*time.Millisecond)
	}, pcapfix.FixWithContext)
	if err != nil {
		return nil, fmt.Errorf("pcapdb: open capture %q: %w", device, err)
	}
	defer h.Close()
	if config.BPF != "" {
		if err = h.SetBPFFilter(config.BPF); err != nil {
			return nil, fmt.Errorf("pcapdb: capture BPF: %w", err)
		}
	}
	if config.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Duration)
		defer cancel()
	}
	w, err := m.CreateLiveDataset(ctx, config, h.LinkType())
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, w.finish(resultErr))
		final := w.Metadata()
		meta = &final
	}()
	if onStarted != nil {
		onStarted(w.Metadata())
	}
	for ctx.Err() == nil {
		select {
		case <-w.done:
			return nil, w.closeErr
		default:
		}
		raw, ci, err := h.ReadPacketData()
		if errors.Is(err, pcap.NextErrorTimeoutExpired) {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if err = w.Feed(raw, ci); err != nil {
			if ctx.Err() != nil {
				return nil, nil
			}
			return nil, err
		}
	}
	return nil, nil
}

// Only permission failures enter the existing authorization flow. Retrying
// once also verifies that authorization actually made this process able to
// capture; a successful repair command alone is not capture readiness.
func openLiveWithPermission(ctx context.Context, open func() (*pcap.Handle, error), authorize func(context.Context) error) (*pcap.Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handle, err := open()
	if err == nil || !isCapturePermissionError(err) {
		return handle, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	authErr := authorize(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if authErr != nil {
		return nil, errors.Join(err, fmt.Errorf("capture permission authorization failed: %w", authErr))
	}
	handle, retryErr := open()
	if retryErr != nil {
		return nil, errors.Join(err, fmt.Errorf("open capture after permission authorization: %w", retryErr))
	}
	return handle, nil
}

func isCapturePermissionError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	// libpcap/Npcap normally return untyped native error strings.
	message := strings.ToLower(err.Error())
	for _, text := range []string{"permission denied", "operation not permitted", "don't have permission", "access is denied", "access denied", "requires superuser"} {
		if strings.Contains(message, text) {
			return true
		}
	}
	return false
}
