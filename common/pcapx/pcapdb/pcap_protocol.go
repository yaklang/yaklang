package pcapdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func (m *InstanceManager) indexProtocols(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata, config *importConfig) (resultErr error) {
	if err := m.transition(ctx, db, meta, StateAnalyzing); err != nil {
		return err
	}
	config.notify(meta)
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
	sessions := newSessionIndexer(meta)
	var batchBytes int
	hasher := sha256.New()
	messages := make([]PCAPProtocolMessage, 0, min(config.batchSize, 2000))
	references := make([]PCAPMessagePacket, 0)
	var lastCatalog time.Time
	commit := func(final bool) error {
		tx := db.BeginTx(ctx, nil)
		if tx.Error != nil {
			return tx.Error
		}
		defer tx.Rollback()
		scoped := indexWithContext(ctx, tx)
		if err := sessions.flush(ctx, scoped); err != nil {
			return err
		}
		if err := scoped.CreateInBatches(&messages).Error; err != nil {
			return err
		}
		if len(messages) > 0 {
			// Convert the new batch once, inside the same transaction. GORM v1
			// batch Create does not support per-row SQL expressions.
			if err := scoped.Model(&PCAPProtocolMessage{}).Where("id >= ? AND id <= ?", messages[0].ID, messages[len(messages)-1].ID).UpdateColumns(map[string]interface{}{
				"fields":       gorm.Expr("jsonb(CAST(fields AS TEXT))"),
				"session":      gorm.Expr("jsonb(CAST(session AS TEXT))"),
				"source_bytes": gorm.Expr("jsonb(CAST(source_bytes AS TEXT))"),
			}).Error; err != nil {
				return err
			}
		}
		if err := scoped.CreateInBatches(&references).Error; err != nil {
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
		// Release variable-sized JSON slices between batches.
		clear(messages)
		messages = messages[:0]
		references = references[:0]
		batchBytes = 0
		sessions.clearBatch()
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
			if config.streams {
				sessionID, streamID, err = sessions.association(ctx, readerDB, event)
				if err != nil {
					return err
				}
			}
			hasher.Write(event.Raw)
			id := meta.ProtocolCount + 1
			messages = append(messages, PCAPProtocolMessage{
				Model:           gorm.Model{ID: uint(id)},
				EventID:         int64(event.ID),
				FlowID:          int64(event.FlowID),
				TimestampNS:     timestampNano(event.Timestamp),
				Protocol:        event.Protocol,
				Transport:       event.Transport,
				Source:          event.Source,
				Destination:     event.Destination,
				Direction:       int(event.Direction),
				LogicalOffset:   int64(event.Offset),
				Length:          event.Length,
				Status:          event.Status,
				Summary:         event.Summary,
				Error:           event.Error,
				DecodeError:     decodeError,
				Section:         int64(event.Domain.Section),
				Interface:       event.Domain.Interface,
				Encapsulation:   event.Domain.Encapsulation,
				Profile:         event.Profile,
				Rule:            event.Rule,
				Entry:           event.Entry,
				Completeness:    event.Completeness,
				ExpertCode:      event.ExpertCode,
				TransactionID:   int64(event.TransactionID),
				ResponseTo:      int64(event.ResponseTo),
				Data:            append([]byte{}, event.Raw...),
				DataLength:      len(event.Raw),
				SessionID:       sessionID,
				StreamID:        streamID,
				FieldsJSON:      fieldsJSON,
				SessionJSON:     sessionJSON,
				SourceBytesJSON: sourceJSON,
			})
			seen := make(map[uint64]bool, len(event.SourceBytes.PacketRefs))
			for _, packet := range event.SourceBytes.PacketRefs {
				if packet.Number == 0 || packet.Number > uint64(meta.PacketCount) {
					return fmt.Errorf("pcapdb: protocol refers to unknown packet %d", packet.Number)
				}
				if !seen[packet.Number] {
					references = append(references, PCAPMessagePacket{MessageID: uint(id), PacketID: uint(packet.Number)})
					seen[packet.Number] = true
				}
			}
			meta.ProtocolCount++
			meta.ProtocolDataSize += int64(len(event.Raw))
			batchBytes += len(event.Raw) + len(fieldsJSON) + len(sessionJSON) + len(sourceJSON)
			if len(messages) >= config.batchSize || len(references) >= config.batchSize || batchBytes+sessions.bytes >= maxImportBatchBytes {
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
		if consumerErr == nil && (sessions.pending() >= config.batchSize || batchBytes+sessions.bytes >= maxImportBatchBytes) {
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
		meta.ProtocolDataSHA256 = hex.EncodeToString(hasher.Sum(nil))
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
