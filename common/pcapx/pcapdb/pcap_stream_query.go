package pcapdb

import (
	"fmt"

	"github.com/yaklang/gorm"
)

type StreamDataChunk struct {
	ID                 uint   `json:"id"`
	ByteOffset         int64  `json:"byte_offset"`
	ReferencesComplete bool   `json:"references_complete"`
	Sequence           uint32 `json:"sequence"`
	TimestampNS        *int64 `json:"timestamp_ns"`
	Data               []byte `json:"data"`
}
type StreamPage struct {
	StreamID  uint              `json:"stream_id"`
	Chunks    []StreamDataChunk `json:"chunks"`
	NextAfter int64             `json:"next_after"`
	HasMore   bool              `json:"has_more"`
	Bytes     int               `json:"bytes"`
}

func parseSummaryQuery(options []QueryOption) (*queryConfig, error) {
	c, err := parseQuery(options, false)
	if err != nil {
		return nil, err
	}
	if c.includeFields {
		return nil, fmt.Errorf("pcapdb: field projection applies to protocol queries")
	}
	return c, nil
}

// QuerySessions returns only bounded summaries. TCP connection reuse and capture
// domains stay distinct; Complete/HasGaps/Midstream describe observed evidence.
func (d *Database) QuerySessions(options ...QueryOption) ([]PCAPSession, error) {
	return d.querySessions(0, options...)
}

// PacketSessions follows the reverse packet/session index (including retransmits).
func (d *Database) PacketSessions(packetID uint, options ...QueryOption) ([]PCAPSession, error) {
	if packetID == 0 {
		return nil, ErrNotFound
	}
	return d.querySessions(packetID, options...)
}
func (d *Database) querySessions(packetID uint, options ...QueryOption) ([]PCAPSession, error) {
	c, err := parseSummaryQuery(options)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	copyConfig := *c
	copyConfig.start = nil
	query := copyConfig.apply(indexWithContext(ctx, tx))
	if c.start != nil {
		query = query.Where("first_timestamp_ns >= ? AND first_timestamp_ns < ?", *c.start, *c.end)
	}
	if c.session != nil {
		query = query.Where("id = ?", *c.session)
	}
	if c.stream != nil {
		query = query.Where("id IN (SELECT session_id FROM streams WHERE id = ? AND deleted_at IS NULL)", *c.stream)
	}
	if packetID != 0 {
		query = query.Where("id IN (SELECT session_id FROM session_packets WHERE packet_id = ? AND deleted_at IS NULL)", packetID)
	}
	rows := make([]PCAPSession, 0)
	err = query.Find(&rows).Error
	return rows, err
}
func (d *Database) QueryStreams(options ...QueryOption) ([]PCAPStream, error) {
	c, err := parseSummaryQuery(options)
	if err != nil {
		return nil, err
	}
	if c.transport != "" || c.sourceIP != "" || c.destinationIP != "" || c.sourcePort != nil || c.destinationPort != nil || c.start != nil {
		return nil, fmt.Errorf("pcapdb: endpoint/time filters apply to session/packet queries")
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	query := indexWithContext(ctx, tx).Where("id > ?", c.after).Order("id").Limit(c.limit)
	if c.session != nil {
		query = query.Where("session_id = ?", *c.session)
	}
	if c.stream != nil {
		query = query.Where("id = ?", *c.stream)
	}
	rows := make([]PCAPStream, 0)
	err = query.Find(&rows).Error
	return rows, err
}

// ReadStream returns complete ordered chunks, capped at 256 KiB by default and
// 1 MiB maximum. Follow NextAfter only while HasMore is true. It never returns a
// concatenated full stream; UDP chunk boundaries identify separate datagrams.
func (d *Database) ReadStream(streamID uint, options ...QueryOption) (*StreamPage, error) {
	c, err := parseSummaryQuery(options)
	if err != nil {
		return nil, err
	}
	if streamID == 0 {
		return nil, ErrNotFound
	}
	if c.transport != "" || c.sourceIP != "" || c.destinationIP != "" || c.sourcePort != nil || c.destinationPort != nil || c.start != nil || c.session != nil || c.stream != nil {
		return nil, fmt.Errorf("pcapdb: ReadStream accepts only context/after/limit/maxBytes options")
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var stream PCAPStream
	err = indexWithContext(ctx, tx).Where("id = ?", streamID).First(&stream).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	page := &StreamPage{StreamID: streamID, Chunks: make([]StreamDataChunk, 0), NextAfter: c.after}
	rows, err := indexWithContext(ctx, tx).Model(&PCAPStreamChunk{}).
		Select("id,byte_offset,sequence,timestamp_ns,length,references_complete,CASE WHEN length(data) <= 65536 THEN data END").
		Where("stream_id = ? AND id > ?", streamID, c.after).Order("id").Limit(c.limit + 1).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var chunk StreamDataChunk
		var length int
		if err = rows.Scan(&chunk.ID, &chunk.ByteOffset, &chunk.Sequence, &chunk.TimestampNS, &length, &chunk.ReferencesComplete, &chunk.Data); err != nil {
			return nil, err
		}
		if chunk.Data == nil || length != len(chunk.Data) || length <= 0 || length > streamChunkBytes {
			return nil, fmt.Errorf("pcapdb: invalid stream chunk BLOB")
		}
		if len(page.Chunks) >= c.limit || page.Bytes+length > c.maxBytes {
			page.HasMore = true
			break
		}
		page.Chunks = append(page.Chunks, chunk)
		page.Bytes += length
		page.NextAfter = int64(chunk.ID)
	}
	return page, rows.Err()
}

func (d *Database) StreamChunkPacketIDs(chunkID uint, options ...QueryOption) ([]int64, error) {
	c, err := parseSummaryQuery(options)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, false, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, 0)
	err = indexWithContext(ctx, tx).Model(&PCAPStreamChunkPacket{}).Where("chunk_id = ? AND packet_id > ?", chunkID, c.after).Order("packet_id").Limit(c.limit).Pluck("packet_id", &ids).Error
	return ids, err
}
