package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// protocolBatch is shared by offline replay and live capture. All mutation is
// serialized by mu, including callbacks from the reassembler's idle expiry.
type protocolBatch struct {
	mu         sync.Mutex
	ctx        context.Context
	db         *gorm.DB
	meta       *PCAPFileDBMetadata
	config     *importConfig
	sessions   *sessionIndexer
	messages   []PCAPProtocolMessage
	references []PCAPMessagePacket
	bytes      int
	hasher     hash.Hash
	err        error
}

func newProtocolBatch(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata, config *importConfig) *protocolBatch {
	return &protocolBatch{ctx: ctx, db: db, meta: meta, config: config, sessions: newSessionIndexer(meta), hasher: sha256.New()}
}

// appendEvent runs under mu. Session associations consult the current batch
// before the committed database, so references work across commit boundaries.
func (b *protocolBatch) appendEvent(event *pcaputil.ProtocolEvent) error {
	if event.ID > math.MaxInt64 || event.FlowID > math.MaxInt64 || event.Offset > math.MaxInt64 || event.TransactionID > math.MaxInt64 || event.ResponseTo > math.MaxInt64 {
		return fmt.Errorf("pcapdb: protocol identifier exceeds SQLite integer range")
	}
	fields := event.Fields
	var decodeError string
	if fields == nil {
		decoded, err := event.GetFields()
		if err != nil {
			decodeError = err.Error()
		} else {
			fields = decoded
		}
	}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	sessionJSON, err := json.Marshal(event.Session)
	if err != nil {
		return err
	}
	sourceJSON, err := json.Marshal(event.SourceBytes)
	if err != nil {
		return err
	}
	if len(event.Raw) > maxCaptureBlock || len(fieldsJSON)+len(sessionJSON)+len(sourceJSON) > 8<<20 {
		return fmt.Errorf("pcapdb: protocol message or JSON metadata exceeds storage budget")
	}
	var sessionID, streamID uint
	if b.config.streams {
		sessionID, streamID, err = b.sessions.association(b.ctx, b.db, event)
		if err != nil {
			return err
		}
	}
	id := b.meta.ProtocolCount + 1
	b.messages = append(b.messages, PCAPProtocolMessage{
		Model: gorm.Model{ID: uint(id)}, EventID: int64(event.ID), FlowID: int64(event.FlowID), TimestampNS: timestampNano(event.Timestamp),
		Protocol: event.Protocol, Transport: event.Transport, Source: event.Source, Destination: event.Destination, Direction: int(event.Direction), LogicalOffset: int64(event.Offset), Length: event.Length,
		Status: event.Status, Summary: event.Summary, Error: event.Error, DecodeError: decodeError, Section: int64(event.Domain.Section), Interface: event.Domain.Interface, Encapsulation: event.Domain.Encapsulation,
		Profile: event.Profile, Rule: event.Rule, Entry: event.Entry, Completeness: event.Completeness, ExpertCode: event.ExpertCode, TransactionID: int64(event.TransactionID), ResponseTo: int64(event.ResponseTo),
		Data: append([]byte{}, event.Raw...), DataLength: len(event.Raw), SessionID: sessionID, StreamID: streamID, FieldsJSON: fieldsJSON, SessionJSON: sessionJSON, SourceBytesJSON: sourceJSON,
	})
	seen := make(map[uint64]bool, len(event.SourceBytes.PacketRefs))
	for _, packet := range event.SourceBytes.PacketRefs {
		if packet.Number == 0 || packet.Number > uint64(b.meta.PacketCount) {
			return fmt.Errorf("pcapdb: protocol refers to unknown packet %d", packet.Number)
		}
		if !seen[packet.Number] {
			b.references = append(b.references, PCAPMessagePacket{MessageID: uint(id), PacketID: uint(packet.Number)})
			seen[packet.Number] = true
		}
	}
	b.hasher.Write(event.Raw)
	b.meta.ProtocolCount++
	b.meta.ProtocolDataSize += int64(len(event.Raw))
	b.bytes += len(event.Raw) + len(fieldsJSON) + len(sessionJSON) + len(sourceJSON)
	return nil
}

func (b *protocolBatch) flush(ctx context.Context, tx *gorm.DB) error {
	if err := b.sessions.flush(ctx, tx); err != nil {
		return err
	}
	if err := tx.CreateInBatches(&b.messages).Error; err != nil {
		return err
	}
	if len(b.messages) > 0 {
		if err := tx.Model(&PCAPProtocolMessage{}).Where("id >= ? AND id <= ?", b.messages[0].ID, b.messages[len(b.messages)-1].ID).UpdateColumns(map[string]interface{}{
			"fields": gorm.Expr("jsonb(CAST(fields AS TEXT))"), "session": gorm.Expr("jsonb(CAST(session AS TEXT))"), "source_bytes": gorm.Expr("jsonb(CAST(source_bytes AS TEXT))"),
		}).Error; err != nil {
			return err
		}
	}
	return tx.CreateInBatches(&b.references).Error
}

func (b *protocolBatch) clear() {
	clear(b.messages)
	b.messages = b.messages[:0]
	clear(b.references)
	b.references = b.references[:0]
	b.bytes = 0
	b.sessions.clearBatch()
}

func (b *protocolBatch) full() bool {
	return len(b.messages) >= b.config.batchSize || len(b.references) >= b.config.batchSize || b.sessions.pending() >= b.config.batchSize || b.bytes+b.sessions.bytes >= maxImportBatchBytes
}

func (m *InstanceManager) indexProtocols(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata, config *importConfig) (resultErr error) {
	if err := m.transition(ctx, db, meta, StateAnalyzing); err != nil {
		return err
	}
	config.notify(meta)
	meta.TLSKeyLogSHA256 = config.tlsKeyLogSHA256
	meta.ProtocolCount, meta.ProtocolDataSize, meta.ProtocolsIndexed = 0, 0, false
	meta.ProtocolDataSHA256, meta.StreamDataSHA256 = "", ""
	meta.SessionCount, meta.StreamCount, meta.StreamChunkCount, meta.StreamDataSize, meta.StreamsIndexed = 0, 0, 0, 0, false
	reset := db.BeginTx(ctx, nil)
	if reset.Error != nil {
		return reset.Error
	}
	defer reset.Rollback()
	scoped := indexWithContext(ctx, reset)
	for _, model := range []interface{}{&PCAPMessagePacket{}, &PCAPProtocolMessage{}, &PCAPStreamChunkPacket{}, &PCAPSessionPacket{}, &PCAPStreamChunk{}, &PCAPStream{}, &PCAPSession{}} {
		if err := scoped.Unscoped().Where("1 = 1").Delete(model).Error; err != nil {
			return err
		}
	}
	if err := writeManifest(ctx, scoped, meta); err != nil {
		return err
	}
	if err := reset.Commit().Error; err != nil {
		return err
	}
	readerDB, err := openIndex(meta.DatabasePath, false, false)
	if err != nil {
		return err
	}
	defer readerDB.Close()
	capture := newCaptureStoreReader(ctx, readerDB, meta)
	defer capture.Close()
	batch := newProtocolBatch(ctx, readerDB, meta, config)
	sessions := batch.sessions
	var lastCatalog time.Time
	commit := func(final bool) error {
		tx := db.BeginTx(ctx, nil)
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		scoped := indexWithContext(ctx, tx)
		if err := batch.flush(ctx, scoped); err != nil {
			return err
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
		batch.clear()
		return ctx.Err()
	}
	replayCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var consumerErr error
	onEvent := func(event *pcaputil.ProtocolEvent) {
		if consumerErr != nil {
			return
		}
		persist := func() error {
			if err := batch.appendEvent(event); err != nil {
				return err
			}
			if batch.full() {
				return commit(false)
			}
			return nil
		}
		if consumerErr = persist(); consumerErr != nil {
			cancel()
		}
	}
	// One replay worker serializes all database consumers; bounded streaming
	// disables the reassembler's full-stream retention. Slow commits apply
	// backpressure instead of building an unbounded transport/event queue.
	consume := func(work func() error) {
		if consumerErr != nil {
			return
		}
		consumerErr = work()
		if consumerErr == nil && batch.full() {
			consumerErr = commit(false)
		}
		if consumerErr != nil {
			cancel()
		}
	}
	options := []pcaputil.CaptureOption{
		pcaputil.WithContext(replayCtx),
		pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: !config.protocols, Stream: true, Workers: 1, MaxFrameBytes: streamChunkBytes, MaxFlows: maxActiveSessions, MaxPendingBytes: 2 << 20, MaxTotalPendingBytes: 16 << 20, MaxPendingSegments: 4096, MaxTotalPendingSegments: 32768}),
	}
	if config.tlsSecrets != nil {
		options = append(options, pcaputil.WithTLSSecrets(config.tlsSecrets))
	}
	if config.protocols {
		options = append(options, pcaputil.WithOnProtocolMessage(onEvent))
	}
	if config.streams {
		options = append(options,
			pcaputil.WithBeforeTransportPacket(func(packet gopacket.Packet) { consume(func() error { return sessions.datagram(packet) }) }),
			pcaputil.WithOnTrafficFlowPacket(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time) {
				consume(func() error { return sessions.tcpPacket(flow, conn, refs, ts) })
			}),
			pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
				consume(func() error { return sessions.tcpData(flow, conn, frame) })
			}),
			pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
				consume(func() error { sessions.closeTCP(reason, flow); return nil })
			}),
		)
	}
	err = pcaputil.ReplayPcap(capture, options...)
	if consumerErr != nil {
		return consumerErr
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	sessions.finish()
	meta.ProtocolsIndexed, meta.StreamsIndexed = config.protocols, config.streams
	if config.streams {
		meta.StreamDataSHA256 = sessions.digest()
	}
	if config.protocols {
		meta.ProtocolDataSHA256 = hex.EncodeToString(batch.hasher.Sum(nil))
	}
	if err = commit(true); err != nil {
		return err
	}
	// Build optional field indexes after the streaming load, avoiding per-row
	// expression-index maintenance during the initial import.
	if err = ensureProtocolFieldIndexes(ctx, db, config.fieldIndexes); err != nil {
		return err
	}
	return m.transition(ctx, db, meta, StateReady)
}
