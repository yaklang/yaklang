package pcapdb

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// PacketViewFilter filters original captured frames before counting/paging.
// Page queries use EXISTS; protocol-filtered totals count distinct completion
// packets from protocol_messages so multiple messages never duplicate a frame.
type PacketViewFilter struct {
	ID, AfterID                   int64
	Offset, Limit                 int
	SourceIPs, DestinationIPs     []string
	SourcePorts, DestinationPorts []int
	Transports, Protocols         []string
	LinkTypes                     []int
	IPVersions                    []int
	StartNS, EndNS                *int64
	Keyword                       string
	SkipTotal                     bool
	ForceTotal                    bool
}

type PacketView struct {
	Packet
	Protocol         string
	Info             string
	RelatedProtocols []string
}

type PacketViewPage struct {
	Packets    []PacketView
	Total      int64
	TotalKnown bool
	NextCursor int64
	HasMore    bool
}

func (f PacketViewFilter) apply(db *gorm.DB) *gorm.DB {
	return f.applyFilters(db.Model(&Packet{}), true)
}

func (f PacketViewFilter) applyFilters(q *gorm.DB, matchProtocols bool) *gorm.DB {
	if f.ID > 0 {
		q = q.Where("packets.id = ?", f.ID)
	}
	for _, term := range []struct {
		column string
		values []string
	}{
		{"src_ip", f.SourceIPs}, {"dst_ip", f.DestinationIPs}, {"transport", f.Transports},
	} {
		if len(term.values) > 0 {
			q = q.Where(term.column+" IN (?)", term.values)
		}
	}
	for _, term := range []struct {
		column string
		values []int
	}{
		{"src_port", f.SourcePorts}, {"dst_port", f.DestinationPorts}, {"link_type", f.LinkTypes},
	} {
		if len(term.values) > 0 {
			q = q.Where(term.column+" IN (?)", term.values)
		}
	}
	if len(f.IPVersions) == 1 {
		if f.IPVersions[0] == 6 {
			q = q.Where("instr(src_ip, ':') > 0")
		} else {
			q = q.Where("instr(src_ip, '.') > 0 AND instr(src_ip, ':') = 0")
		}
	}
	if len(f.IPVersions) == 2 {
		q = q.Where("src_ip <> ''")
	}
	if f.StartNS != nil {
		q = q.Where("timestamp_ns >= ?", *f.StartNS)
	}
	if f.EndNS != nil {
		q = q.Where("timestamp_ns < ?", *f.EndNS)
	}
	const associated = "SELECT 1 FROM message_packets mp INDEXED BY message_packets_packet JOIN protocol_messages pm ON pm.id=mp.message_id WHERE mp.packet_id=packets.id AND mp.deleted_at IS NULL AND pm.deleted_at IS NULL"
	if matchProtocols && len(f.Protocols) > 0 {
		if q.Dialect().HasColumn("protocol_messages", "completed_in_packet_id") {
			// Match Wireshark's application protocol display filter: only the frame
			// where the complete message is dissected belongs to that protocol.
			q = q.Where("EXISTS (SELECT 1 FROM protocol_messages pm INDEXED BY messages_completed_in WHERE pm.completed_in_packet_id=packets.id AND pm.deleted_at IS NULL AND pm.protocol IN (?))", f.Protocols)
		} else {
			// Legacy datasets lack the direct completion column. Match the same
			// Wireshark-style completion semantics from the preserved evidence.
			q = q.Where("EXISTS ("+associated+" AND pm.protocol IN (?) AND CAST(json_extract(pm.source_bytes,'$.CompletedIn.Number') AS INTEGER)=packets.id)", f.Protocols)
		}
	}
	if f.Keyword != "" {
		// instr treats %, _ and quotes literally, unlike an unescaped LIKE.
		key := strings.ToLower(f.Keyword)
		clause := "instr(lower(src_ip), ?) > 0 OR instr(lower(dst_ip), ?) > 0 OR instr(lower(transport), ?) > 0"
		args := []interface{}{key, key, key}
		if q.Dialect().HasColumn("packets", "summary") {
			clause += " OR instr(lower(packets.summary), ?) > 0"
			args = append(args, key)
		}
		if q.Dialect().HasColumn("protocol_messages", "completed_in_packet_id") {
			clause += " OR EXISTS (SELECT 1 FROM protocol_messages pm INDEXED BY messages_completed_in WHERE pm.completed_in_packet_id=packets.id AND pm.deleted_at IS NULL AND (instr(lower(pm.summary), ?) > 0 OR instr(lower(pm.protocol), ?) > 0))"
		} else {
			clause += " OR EXISTS (" + associated + " AND CAST(json_extract(pm.source_bytes,'$.CompletedIn.Number') AS INTEGER)=packets.id AND (instr(lower(pm.summary), ?) > 0 OR instr(lower(pm.protocol), ?) > 0))"
		}
		args = append(args, key, key)
		q = q.Where("("+clause+")", args...)
	}
	return q
}

func (f PacketViewFilter) count(db *gorm.DB) (int64, error) {
	if q, ok := f.countQuery(db); ok {
		var total int64
		err := q.Count(&total).Error
		return total, err
	}
	var total int64
	err := f.apply(db).Count(&total).Error
	return total, err
}

func (f PacketViewFilter) countQuery(db *gorm.DB) (*gorm.DB, bool) {
	if len(f.Protocols) == 0 || !db.Dialect().HasColumn("protocol_messages", "completed_in_packet_id") {
		return nil, false
	}

	// Page queries must walk packets in ID order so they can stop at LIMIT.
	// Counting has the opposite optimal direction: enumerate the smaller set of
	// completed protocol messages, de-duplicate their completion frames, then
	// join back to packets for any remaining packet-level predicates.
	completions := db.Table("protocol_messages INDEXED BY messages_protocol_completed").
		Select("DISTINCT completed_in_packet_id AS packet_id").
		Where("deleted_at IS NULL AND completed_in_packet_id > 0 AND protocol IN (?)", f.Protocols)
	q := db.Table("packets").
		Joins("JOIN (?) AS protocol_completions ON protocol_completions.packet_id=packets.id", completions.QueryExpr()).
		Where("packets.deleted_at IS NULL")
	q = f.applyFilters(q, false)
	return q, true
}

func (h *DatabaseHandle) QueryPacketView(filter PacketViewFilter) (*PacketViewPage, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	d := h.database
	if filter.ID < 0 || filter.AfterID < 0 || filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 1000 {
		return nil, fmt.Errorf("pcapdb: invalid packet view pagination")
	}
	if err := d.lockRead(h.ctx); err != nil {
		return nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	tx, meta, err := d.readSnapshot(h.ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if len(filter.Protocols) > 0 && !meta.ProtocolsIndexed {
		return nil, fmt.Errorf("%w: protocol analysis unavailable; reimport the capture to build its protocol index", ErrNotReady)
	}
	scoped := indexWithContext(h.ctx, tx)
	page := &PacketViewPage{Packets: make([]PacketView, 0), NextCursor: filter.AfterID}
	// A cursor page must not repeat a full filtered COUNT. The first page (or
	// an exact ID lookup) carries the total; subsequent pages re-use it on the
	// client and keep only the bounded page query on the database.
	if !filter.SkipTotal && filter.Offset == 0 && (filter.AfterID == 0 || filter.ForceTotal) {
		page.Total, err = filter.count(scoped)
		if err != nil {
			return nil, err
		}
		page.TotalKnown = true
	}
	var packets []Packet
	err = filter.apply(scoped).Where("packets.id > ?", filter.AfterID).
		Select(packetColumns(scoped, "packets.", false)).Order("packets.id").Offset(filter.Offset).Limit(filter.Limit + 1).Find(&packets).Error
	if err != nil {
		return nil, err
	}
	page.HasMore = len(packets) > filter.Limit
	if page.HasMore {
		packets = packets[:filter.Limit]
	}
	for _, p := range packets {
		page.Packets = append(page.Packets, PacketView{Packet: p, Info: p.Summary})
		page.NextCursor = int64(p.ID)
	}
	if len(packets) == 0 || !meta.ProtocolsIndexed {
		return page, nil
	}
	// Batch-load small summaries for this page only. No message fields/raw BLOBs.
	// They are supplementary evidence: a captured TCP segment's own summary must
	// remain authoritative even if the segment helps reassemble an HTTP message.
	ids := make([]uint, 0, len(packets))
	positions := make(map[uint]int, len(packets))
	for i, p := range packets {
		ids = append(ids, p.ID)
		positions[p.ID] = i
	}
	var summaries []struct {
		PacketID          uint
		Protocol, Summary string
		CompletedIn       uint
	}
	if scoped.Dialect().HasColumn("protocol_messages", "completed_in_packet_id") {
		// Lists only need messages completed by this page. Contributor frames
		// keep their TCP/UDP summary; complete messages remain available from
		// Details and via ApplicationLayerTypes predicate filtering.
		summariesQuery := scoped.Table("protocol_messages INDEXED BY messages_completed_in").
			Select("completed_in_packet_id AS packet_id,protocol,substr(summary,1,512) AS summary,completed_in_packet_id AS completed_in").
			Where("completed_in_packet_id IN (?) AND deleted_at IS NULL", ids)
		if len(filter.Protocols) > 0 {
			summariesQuery = summariesQuery.Where("protocol IN (?)", filter.Protocols)
		}
		err = summariesQuery.Order("id").Limit(10001).Scan(&summaries).Error
	} else {
		completedInColumn := "CAST(json_extract(pm.source_bytes,'$.CompletedIn.Number') AS INTEGER)"
		// Without the explicit index, SQLite may choose the deleted_at index
		// and scan every live relation before applying packet_id IN (...).
		summariesQuery := scoped.Table("message_packets AS mp INDEXED BY message_packets_packet").
			Joins("JOIN protocol_messages AS pm ON pm.id=mp.message_id").
			Select(fmt.Sprintf("mp.packet_id,pm.protocol,substr(pm.summary,1,512) AS summary,%s AS completed_in", completedInColumn)).
			Where("mp.packet_id IN (?) AND mp.deleted_at IS NULL AND pm.deleted_at IS NULL AND "+completedInColumn+"=mp.packet_id", ids)
		if len(filter.Protocols) > 0 {
			summariesQuery = summariesQuery.Where("pm.protocol IN (?)", filter.Protocols)
		}
		err = summariesQuery.Order("pm.id").Limit(10001).Scan(&summaries).Error
	}
	if err != nil {
		return nil, err
	}
	if len(summaries) > 10000 {
		return nil, ErrResultTooLarge
	}
	for _, m := range summaries {
		p := &page.Packets[positions[m.PacketID]]
		protocol := strings.ToUpper(m.Protocol)
		if !lo.Contains(p.RelatedProtocols, protocol) {
			p.RelatedProtocols = append(p.RelatedProtocols, protocol)
		}
		if m.CompletedIn == p.ID && p.Protocol == "" {
			// Match Wireshark's list convention: the frame that completes a
			// reassembled PDU is dissected as that application protocol and gets
			// the protocol message's Info text.
			p.Protocol = protocol
			p.Info = shortText(m.Summary, 512)
		}
	}
	return page, nil
}

// PacketMessages reads saved analysis, not a second on-demand reassembly.
// Both BLOB and JSON budgets are checked before materializing message content.
func (h *DatabaseHandle) PacketMessages(id uint, budget int) ([]ProtocolMessage, error) {
	if err := h.check(); err != nil {
		return nil, err
	}
	d := h.database
	if err := d.lockRead(h.ctx); err != nil {
		return nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	tx, meta, err := d.readSnapshot(h.ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	messages := make([]ProtocolMessage, 0)
	if !meta.ProtocolsIndexed {
		return messages, nil
	}
	scoped := indexWithContext(h.ctx, tx)
	// A terminal frame is authoritative even if a parser did not also list it
	// among the byte-contributing PacketRefs. Contributor frames still resolve
	// through message_packets.
	q := scoped.Model(&PCAPProtocolMessage{}).Where(
		"completed_in_packet_id = ? OR EXISTS (SELECT 1 FROM message_packets mp WHERE mp.message_id=protocol_messages.id AND mp.packet_id=? AND mp.deleted_at IS NULL)",
		id,
		id,
	)
	var size struct {
		Bytes int64
		Count int
	}
	err = q.Select("COALESCE(sum(length(data)+length(json(fields))+length(json(source_bytes))),0) AS bytes,count(*) AS count").Scan(&size).Error
	if err != nil {
		return nil, err
	}
	if size.Bytes > int64(budget) || size.Count > 1000 {
		return nil, ErrResultTooLarge
	}
	err = q.Select(protocolColumns(scoped, false) + ",data,json(fields) AS fields,json(source_bytes) AS source_bytes").Order("id").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	for i := range messages {
		if err := decodeProtocolFields(messages[i].FieldsJSON, &messages[i].Fields); err != nil {
			return nil, err
		}
	}
	return messages, nil
}

// MessageByteSource preserves capture frame identities and completion semantics.
func MessageByteSource(message ProtocolMessage) (pcaputil.ByteSource, error) {
	var source pcaputil.ByteSource
	err := json.Unmarshal(message.SourceBytesJSON, &source)
	return source, err
}
