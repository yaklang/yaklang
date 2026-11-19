package pcapdb

import (
	"errors"
	"fmt"

	"github.com/yaklang/gorm"
)

const MaxByteReadLength = 8 << 20

var ErrRangeOutOfBounds = errors.New("pcapdb: byte range is outside the selected source")

// DataRange is a bounded byte interval from one stored packet or protocol
// message. EOF describes this source only; it does not claim protocol
// completeness when reassembly itself was incomplete.
type DataRange struct {
	Data        []byte
	Offset      int64
	TotalLength int64
	EOF         bool
}

func (d *Database) ReadPacketRange(id uint, offset int64, length int, options ...QueryOption) (*DataRange, error) {
	return d.readDataRange(id, false, offset, length, options...)
}

func (d *Database) ReadProtocolRange(id uint, offset int64, length int, options ...QueryOption) (*DataRange, error) {
	return d.readDataRange(id, true, offset, length, options...)
}

func (d *Database) readDataRange(id uint, protocol bool, offset int64, length int, options ...QueryOption) (*DataRange, error) {
	if id == 0 {
		return nil, ErrNotFound
	}
	if offset < 0 || length < 1 || length > MaxByteReadLength {
		return nil, fmt.Errorf("pcapdb: byte ranges require offset >= 0 and length in 1..%d", MaxByteReadLength)
	}
	c, err := parseDetailQuery(options)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	tx, _, err := d.readSnapshot(ctx, protocol, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	model := any(&Packet{})
	lengthColumn := "captured_length"
	if protocol {
		model = &PCAPProtocolMessage{}
		lengthColumn = "data_length"
	}
	var row struct {
		ID     uint
		Total  int64
		Actual int64
		Data   []byte
	}
	err = indexWithContext(ctx, tx).Model(model).
		Select("id,"+lengthColumn+" AS total,length(data) AS actual,substr(data,?,?) AS data", offset+1, length).
		Where("id = ?", id).Limit(1).Scan(&row).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	if row.Total < 0 || row.Total > 16<<20 || row.Total != row.Actual {
		return nil, fmt.Errorf("pcapdb: invalid packet/protocol BLOB length")
	}
	if offset > row.Total {
		return nil, fmt.Errorf("%w: offset %d exceeds length %d", ErrRangeOutOfBounds, offset, row.Total)
	}
	if row.Data == nil {
		row.Data = []byte{}
	}
	if len(row.Data) > length || offset+int64(len(row.Data)) > row.Total {
		return nil, fmt.Errorf("pcapdb: SQLite returned an invalid byte range")
	}
	return &DataRange{
		Data: row.Data, Offset: offset, TotalLength: row.Total,
		EOF: offset+int64(len(row.Data)) == row.Total,
	}, nil
}

func (h *DatabaseHandle) ReadPacketRange(id uint, offset int64, length int, options ...QueryOption) (*DataRange, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ReadPacketRange(id, offset, length, options...)
}

func (h *DatabaseHandle) ReadProtocolRange(id uint, offset int64, length int, options ...QueryOption) (*DataRange, error) {
	options, err := h.options(options)
	if err != nil {
		return nil, err
	}
	return h.database.ReadProtocolRange(id, offset, length, options...)
}
