package pcapdb

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/ytoken"
)

const (
	maxProtocolDetailBatchIDs = 50

	// Detail batches are rendered inside the AI tool result envelope. Keep
	// enough room for RESULT/OBSERVATIONS labels, artifact paths and outer
	// truncation bookkeeping so the canonical tool limit never has to cut the
	// JSON body itself.
	maxProtocolDetailBatchTokens = 10 * 1024
	protocolDetailBatchReserve   = 6 * 1024
)

// ProtocolDetailBatchItem is one requested protocol detail. Per-record failures
// stay attached to the requested ID so one missing or oversized item cannot hide
// successful records from the same batch.
type ProtocolDetailBatchItem struct {
	ID            uint           `json:"id"`
	Protocol      string         `json:"protocol,omitempty"`
	Transport     string         `json:"transport,omitempty"`
	Status        string         `json:"status,omitempty"`
	Completeness  string         `json:"completeness,omitempty"`
	SessionID     uint           `json:"session_id,omitempty"`
	StreamID      uint           `json:"stream_id,omitempty"`
	TransactionID int64          `json:"transaction_id,omitempty"`
	ResponseTo    int64          `json:"response_to,omitempty"`
	DataLength    int            `json:"data_length,omitempty"`
	Fields        map[string]any `json:"fields,omitempty"`
	Session       map[string]any `json:"session,omitempty"`
	Error         *ResultError   `json:"error,omitempty"`
}

type protocolDetailBatchRow struct {
	ID            uint
	Protocol      string
	Transport     string
	Status        string
	Completeness  string
	SessionID     uint
	StreamID      uint
	TransactionID int64
	ResponseTo    int64
	DataLength    int
	FieldsJSON    []byte `gorm:"column:fields"`
	SessionJSON   []byte `gorm:"column:session"`
}

// ProtocolDetailsBatchPage returns details in exactly the requested ID order.
// It uses one read snapshot and one bounded IN query instead of opening one
// detail transaction per record. Duplicate and zero IDs are rejected so the
// request cost remains explicit and stable.
func (d *Database) ProtocolDetailsBatchPage(ids []uint, options ...QueryOption) (*ResultPage[ProtocolDetailBatchItem], error) {
	page := emptyPage[ProtocolDetailBatchItem](d.ID, 0, "record_id_batch")
	c, err := parseDetailQuery(options)
	if err != nil {
		return failPage(page, err)
	}
	if len(ids) == 0 {
		return failPage(page, fmt.Errorf("pcapdb: record_ids must contain at least one ID"))
	}
	if len(ids) > maxProtocolDetailBatchIDs {
		return failPage(page, fmt.Errorf("pcapdb: record_ids accepts at most %d IDs", maxProtocolDetailBatchIDs))
	}
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return failPage(page, fmt.Errorf("pcapdb: record_ids must contain positive IDs"))
		}
		if _, ok := seen[id]; ok {
			return failPage(page, fmt.Errorf("pcapdb: duplicate record ID %d", id))
		}
		seen[id] = struct{}{}
	}

	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, true, false)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()

	rows := make([]protocolDetailBatchRow, 0, len(ids))
	err = indexWithContext(ctx, tx).Model(&PCAPProtocolMessage{}).
		Select(fmt.Sprintf(
			"id,protocol,transport,status,completeness,session_id,stream_id,transaction_id,response_to,data_length,"+
				"CASE WHEN length(fields)+length(session) <= %d THEN json(fields) END AS fields,"+
				"CASE WHEN length(fields)+length(session) <= %d THEN json(session) END AS session",
			c.resultBytes, c.resultBytes,
		),
		).
		Where("id IN (?)", ids).
		Scan(&rows).Error
	if err != nil {
		return failPage(page, err)
	}
	byID := make(map[uint]protocolDetailBatchRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}

	page.State = StateReady
	items := make([]ProtocolDetailBatchItem, 0, len(ids))
	for _, id := range ids {
		item := ProtocolDetailBatchItem{ID: id}
		row, ok := byID[id]
		if !ok {
			item.Error = resultError(fmt.Errorf("%w: protocol record %d", ErrNotFound, id))
		} else {
			item.Protocol = shortText(row.Protocol, 128)
			item.Transport = shortText(row.Transport, 32)
			item.Status = shortText(row.Status, 128)
			item.Completeness = shortText(row.Completeness, 128)
			item.SessionID = row.SessionID
			item.StreamID = row.StreamID
			item.TransactionID = row.TransactionID
			item.ResponseTo = row.ResponseTo
			item.DataLength = row.DataLength
			if row.FieldsJSON == nil || row.SessionJSON == nil {
				item.Error = resultError(ErrResultTooLarge)
			} else if err := decodeProtocolFields(row.FieldsJSON, &item.Fields); err != nil {
				item.Error = resultError(fmt.Errorf("pcapdb: decode fields for record %d: %w", id, err))
			} else if err := decodeProtocolFields(row.SessionJSON, &item.Session); err != nil {
				item.Error = resultError(fmt.Errorf("pcapdb: decode session for record %d: %w", id, err))
				item.Fields = nil
			}
		}
		if item.Error != nil && item.Error.Code == "result_too_large" {
			page.Truncated = true
		}
		items = append(items, item)
		page.NextCursor = int64(id)
	}
	page.Items = items
	fits, err := fitProtocolDetailBatchPage(page, c.resultBytes, maxProtocolDetailBatchTokens)
	if err != nil {
		return failPage(page, err)
	}
	if !fits {
		return failPage(page, fmt.Errorf("%w: detail batch metadata exceeds %d bytes or %d tokens", ErrResultTooLarge, c.resultBytes, maxProtocolDetailBatchTokens))
	}
	return page, nil
}

func fitProtocolDetailBatchPage(
	page *ResultPage[ProtocolDetailBatchItem],
	byteBudget int,
	tokenBudget int,
) (bool, error) {
	fits, err := protocolDetailBatchPageFits(page, byteBudget, tokenBudget)
	if err != nil {
		return false, err
	}
	for !fits {
		largest := -1
		largestBytes := 0
		for i := range page.Items {
			item := page.Items[i]
			if item.Error != nil || len(item.Fields) == 0 && len(item.Session) == 0 {
				continue
			}
			encoded, err := json.Marshal(item)
			if err != nil {
				return false, err
			}
			if len(encoded) > largestBytes {
				largest, largestBytes = i, len(encoded)
			}
		}
		if largest < 0 {
			return false, nil
		}
		item := &page.Items[largest]
		item.Fields = nil
		item.Session = nil
		item.Error = resultError(fmt.Errorf(
			"%w: batch prompt budget exhausted for record %d; retry this ID in a smaller batch",
			ErrResultTooLarge,
			item.ID,
		))
		page.Truncated = true
		fits, err = protocolDetailBatchPageFits(page, byteBudget, tokenBudget)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func protocolDetailBatchPageFits(page *ResultPage[ProtocolDetailBatchItem], byteBudget, tokenBudget int) (bool, error) {
	encoded, err := json.Marshal(page)
	if err != nil {
		return false, err
	}
	if len(encoded) > byteBudget {
		return false, nil
	}
	return !ytoken.TokenCountExceeds(string(encoded), tokenBudget), nil
}
