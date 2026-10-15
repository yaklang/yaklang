package pcapdb

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/yaklang/gorm"
)

const maxImportBatchBytes = 8 << 20
const captureMetadataChunkBytes = 1 << 20

// captureStoreReader reconstructs the original capture from framing and packet
// BLOBs. It holds at most one bounded record, never the whole file. Each page
// ends its read snapshot so protocol replay cannot pin the writer's entire WAL.
// Its caller holds the dataset lock to prevent destructive rebuilds between pages.
type captureStoreReader struct {
	ctx       context.Context
	db        *gorm.DB
	meta      *PCAPFileDBMetadata
	rows      *sql.Rows
	after     int64
	pageStart int64
	bytes     int64
	parts     [][]byte
	err       error
}

func newCaptureStoreReader(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata) *captureStoreReader {
	return &captureStoreReader{ctx: ctx, db: db, meta: meta}
}

func (r *captureStoreReader) Close() error {
	if r.rows != nil {
		err := r.rows.Close()
		r.rows = nil
		return err
	}
	return nil
}

func (r *captureStoreReader) next() error {
	for {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if r.rows == nil {
			rows, err := indexWithContext(r.ctx, r.db).Table("capture_records AS cr").
				Joins("LEFT JOIN packets AS p ON p.id = cr.packet_id AND p.deleted_at IS NULL").
				Select("cr.id,cr.packet_id,CASE WHEN length(cr.prefix) <= 16777216 THEN cr.prefix END,CASE WHEN length(cr.suffix) <= 16777216 THEN cr.suffix END,p.id,p.captured_length,CASE WHEN length(p.data) <= 16777216 THEN p.data END").
				Where("cr.deleted_at IS NULL AND cr.id > ?", r.after).Order("cr.id").Limit(64).Rows()
			if err != nil {
				return err
			}
			r.rows = rows
			r.pageStart = r.after
		}
		if r.rows.Next() {
			var id int64
			var packetID, foundID, length sql.NullInt64
			var prefix, suffix, data []byte
			if err := r.rows.Scan(&id, &packetID, &prefix, &suffix, &foundID, &length, &data); err != nil {
				return err
			}
			if id != r.after+1 || id > r.meta.CaptureRecordCount || prefix == nil || suffix == nil {
				return fmt.Errorf("pcapdb: invalid capture framing record %d", id)
			}
			if packetID.Valid && (!foundID.Valid || data == nil || !length.Valid || length.Int64 != int64(len(data))) {
				return fmt.Errorf("pcapdb: missing or invalid packet BLOB in capture record %d", id)
			}
			r.after = id
			r.parts = [][]byte{prefix, data, suffix}
			return nil
		}
		err := r.rows.Err()
		closeErr := r.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if r.after == r.meta.CaptureRecordCount {
			if r.bytes != r.meta.BytesIndexed {
				return fmt.Errorf("pcapdb: capture BLOB byte count disagrees with checkpoint")
			}
			return io.EOF
		}
		if r.after == r.pageStart {
			return fmt.Errorf("pcapdb: capture framing records are missing")
		}
	}
}

func (r *captureStoreReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if r.err != nil {
			return 0, r.err
		}
		if err := r.ctx.Err(); err != nil {
			r.err = err
			return 0, err
		}
		for len(r.parts) > 0 {
			if len(r.parts[0]) == 0 {
				r.parts = r.parts[1:]
				continue
			}
			n := copy(p, r.parts[0])
			r.parts[0] = r.parts[0][n:]
			r.bytes += int64(n)
			return n, nil
		}
		if err := r.next(); err != nil {
			r.err = err
			return 0, err
		}
	}
}
