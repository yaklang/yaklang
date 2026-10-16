package ssadb

import (
	"context"
	"database/sql"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
)

// nativeOffsetQueries counts native ir_offsets SELECT statements. Tests use it
// to prove a preload replaces per-variable queries; it is not a performance API.
var nativeOffsetQueries atomic.Int64

// NativeOffsetQueries returns the test-only native offset query counter.
func NativeOffsetQueries() int64 {
	return nativeOffsetQueries.Load()
}

const nativeOffsetSelect = `SELECT id, created_at, updated_at, deleted_at, program_name, file_hash, start_offset, end_offset, variable_name, value_id FROM ` + TableIrOffsets

// nativeOffsetBatchChunk stays under SQLite's historical 999 host-parameter limit
// after adding the program argument.
const nativeOffsetBatchChunk = 500

// nativeGetOffsetsByVariable keeps the existing scalar ir_offsets format and
// soft-delete/program filtering. This is not a new storage version. A direct
// call does not cache misses. Read-mode preload may cache non-empty value
// snapshots separately; callers still receive their own row copies.
func nativeGetOffsetsByVariable(db *gorm.DB, name string, valueID int64, programName string) ([]*IrOffset, error) {
	query := nativeOffsetSelect + ` WHERE deleted_at IS NULL AND variable_name = ? AND value_id = ?`
	args := []any{name, valueID}
	if program := strings.TrimSpace(programName); program != "" {
		query += ` AND program_name = ?`
		args = append(args, program)
	}
	query += ` ORDER BY id`
	return queryOffsetRows(db, query, args...)
}

// nativeGetOffsetsByValueIDs reads every non-deleted binding for the requested
// values. Rows stay in primary-key order. An empty program keeps the historical
// cross-program lookup and must not be cached by the caller.
func nativeGetOffsetsByValueIDs(db *gorm.DB, programName string, valueIDs []int64) ([]*IrOffset, error) {
	clean := compactPositiveIDs(valueIDs)
	if db == nil || len(clean) == 0 {
		return []*IrOffset{}, nil
	}
	var all []*IrOffset
	for start := 0; start < len(clean); start += nativeOffsetBatchChunk {
		end := start + nativeOffsetBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		chunk := clean[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		query := nativeOffsetSelect + ` WHERE deleted_at IS NULL AND value_id IN (`
		for i, id := range chunk {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += joinPlaceholders(placeholders) + `)`
		if program := strings.TrimSpace(programName); program != "" {
			query += ` AND program_name = ?`
			args = append(args, program)
		}
		query += ` ORDER BY id`
		rows, err := queryOffsetRows(db, query, args...)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	if all == nil {
		return []*IrOffset{}, nil
	}
	return all, nil
}

func compactPositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func queryOffsetRows(db *gorm.DB, query string, args ...any) (offsets []*IrOffset, err error) {
	nativeOffsetQueries.Add(1)
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
