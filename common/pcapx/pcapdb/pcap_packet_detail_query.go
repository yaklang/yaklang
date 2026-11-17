package pcapdb

import (
	"fmt"
)

// PacketDetailMessages loads typed display trees and source metadata without
// materializing protocol message BLOBs. Callers read initial or requested
// ranges through the bounded data-range API.
func (h *DatabaseHandle) PacketDetailMessages(packetID uint, budget int) (completed []ProtocolMessage, related []ProtocolMessage, err error) {
	if packetID == 0 {
		return nil, nil, fmt.Errorf("pcapdb: packet ID is required")
	}
	if budget < 1 {
		return nil, nil, fmt.Errorf("pcapdb: protocol detail budget must be positive")
	}
	if err := h.check(); err != nil {
		return nil, nil, err
	}
	d := h.database
	if err := d.lockRead(h.ctx); err != nil {
		return nil, nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, nil, ErrClosed
	}
	tx, meta, err := d.readSnapshot(h.ctx, false, false)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	if !meta.ProtocolsIndexed {
		return []ProtocolMessage{}, []ProtocolMessage{}, nil
	}
	scoped := indexWithContext(h.ctx, tx)
	completedQuery := scoped.Model(&PCAPProtocolMessage{}).Where("completed_in_packet_id = ?", packetID)
	const relatedFrom = " FROM message_packets AS mp INDEXED BY message_packets_packet" +
		" JOIN protocol_messages AS pm ON pm.id = mp.message_id" +
		" WHERE mp.packet_id = ? AND mp.deleted_at IS NULL AND pm.deleted_at IS NULL" +
		" AND pm.completed_in_packet_id <> ?"
	type detailSize struct {
		Bytes int64
		Count int
	}
	var completedSize, relatedSize detailSize
	err = completedQuery.Select("COALESCE(sum(length(fields)+length(display_fields)+length(source_bytes)),0) AS bytes,count(*) AS count").
		Scan(&completedSize).Error
	if err != nil {
		return nil, nil, err
	}
	err = scoped.Raw("SELECT COALESCE(sum(length(pm.fields)+length(pm.display_fields)+length(pm.source_bytes)),0) AS bytes,count(*) AS count"+
		relatedFrom, packetID, packetID).Scan(&relatedSize).Error
	if err != nil {
		return nil, nil, err
	}
	if completedSize.Bytes+relatedSize.Bytes > int64(budget) || completedSize.Count+relatedSize.Count > 1000 {
		return nil, nil, ErrResultTooLarge
	}
	completedSelection := protocolColumns(scoped, false) + ",json(fields) AS fields,json(display_fields) AS display_fields,json(source_bytes) AS source_bytes"
	err = scoped.Model(&PCAPProtocolMessage{}).
		Select(completedSelection).
		Where("completed_in_packet_id = ?", packetID).
		Order("id").Limit(1001).Find(&completed).Error
	if err != nil {
		return nil, nil, err
	}
	relatedSelection := protocolColumnsWithPrefix(scoped, "pm.", false) +
		",json(pm.fields) AS fields,json(pm.display_fields) AS display_fields,json(pm.source_bytes) AS source_bytes"
	err = scoped.Raw("SELECT "+relatedSelection+relatedFrom+" ORDER BY pm.id LIMIT 1001", packetID, packetID).
		Scan(&related).Error
	if err != nil {
		return nil, nil, err
	}
	return completed, related, nil
}
