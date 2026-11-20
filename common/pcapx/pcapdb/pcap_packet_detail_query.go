package pcapdb

import "fmt"

// PacketDetailMessages loads the parser snapshots for completed messages only.
// A contributor needs a link, not the other message's field tree or raw BLOB.
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
	// Charge the UTF-8 JSON bytes that the SELECT below actually returns,
	// not the size of SQLite's compact JSONB or its character count.
	const completedBytes = "length(CAST(json(fields) AS BLOB))+length(CAST(json(session) AS BLOB))+length(CAST(json(source_bytes) AS BLOB))"
	const relatedSource = "CASE WHEN pm.completed_in_packet_id=0 THEN json(pm.source_bytes) END"
	err = completedQuery.Select("COALESCE(sum(" + completedBytes + "),0) AS bytes,count(*) AS count").
		Scan(&completedSize).Error
	if err != nil {
		return nil, nil, err
	}
	err = scoped.Raw("SELECT COALESCE(sum(length(CAST("+relatedSource+" AS BLOB))),0) AS bytes,count(*) AS count"+
		relatedFrom, packetID, packetID).Scan(&relatedSize).Error
	if err != nil {
		return nil, nil, err
	}
	if completedSize.Bytes+relatedSize.Bytes > int64(budget) || completedSize.Count+relatedSize.Count > 1000 {
		return nil, nil, ErrResultTooLarge
	}
	completedSelection := protocolColumns(scoped, false) + ",json(fields) AS fields,json(session) AS session,json(source_bytes) AS source_bytes"
	err = scoped.Model(&PCAPProtocolMessage{}).
		Select(completedSelection).
		Where("completed_in_packet_id = ?", packetID).
		Order("id").Limit(1001).Find(&completed).Error
	if err != nil {
		return nil, nil, err
	}
	for i := range completed {
		message := &completed[i]
		if len(message.FieldsJSON) > 0 {
			if err := decodeProtocolFields(message.FieldsJSON, &message.Fields); err != nil {
				message.DecodeError = appendDecodeDiagnostic(message.DecodeError, "fields", err)
			}
		}
		if len(message.SessionJSON) > 0 {
			if err := decodeProtocolFields(message.SessionJSON, &message.Session); err != nil {
				message.DecodeError = appendDecodeDiagnostic(message.DecodeError, "session", err)
			}
		}
		// The caller receives owned maps. Returning the JSON BLOBs as well would
		// duplicate the entire snapshot for every detail response.
		message.FieldsJSON, message.SessionJSON = nil, nil
	}
	const relatedSelection = "pm.id,pm.protocol,pm.summary,pm.completeness,pm.completed_in_packet_id," + relatedSource + " AS source_bytes"
	err = scoped.Raw("SELECT "+relatedSelection+relatedFrom+" ORDER BY pm.id LIMIT 1001", packetID, packetID).
		Scan(&related).Error
	if err != nil {
		return nil, nil, err
	}
	return completed, related, nil
}

func appendDecodeDiagnostic(existing, label string, err error) string {
	current := label + ": " + err.Error()
	if existing == "" {
		return current
	}
	return existing + "; " + current
}
