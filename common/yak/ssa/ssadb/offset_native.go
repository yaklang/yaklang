package ssadb

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

// nativeGetOffsetsByVariable keeps the existing scalar ir_offsets format and
// soft-delete/program filtering. This is only a reader optimization, not a new
// storage version. Each call owns its rows: compile-time misses are not cached,
// and callers cannot accidentally share mutable variable/location state.
func nativeGetOffsetsByVariable(db *gorm.DB, name string, valueID int64, programName string) (offsets []*IrOffset, err error) {
	query := `SELECT id, created_at, updated_at, deleted_at, program_name, file_hash, start_offset, end_offset, variable_name, value_id FROM ` + TableIrOffsets + ` WHERE deleted_at IS NULL AND variable_name = ? AND value_id = ?`
	args := []any{name, valueID}
	if program := strings.TrimSpace(programName); program != "" {
		query += ` AND program_name = ?`
		args = append(args, program)
	}
	query = bindSQLPlaceholdersDB(db, query)
	started := time.Now()
	defer func() {
		elapsed := time.Since(started)
		RecordDBRead(elapsed, err != nil)
		logNativeSQL(query, elapsed, err)
	}()
	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	rows, err := nativeQueryContextDB(db, ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			offsets, err = nil, closeErr
		}
	}()
	offsets = make([]*IrOffset, 0)
	for rows.Next() {
		offset := new(IrOffset)
		var deletedAt sql.NullTime
		if err = rows.Scan(&offset.ID, &offset.CreatedAt, &offset.UpdatedAt, &deletedAt,
			&offset.ProgramName, &offset.FileHash, &offset.StartOffset, &offset.EndOffset,
			&offset.VariableName, &offset.ValueID); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			offset.DeletedAt = &deletedAt.Time
		}
		offsets = append(offsets, offset)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return offsets, nil
}
