package ssadb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

// secondaryIndexTables are the IR tables a compile writes. Non-unique indexes
// on them turn sequential inserts into random B-tree maintenance. UNIQUE
// indexes are left in place because the insert path depends on them.
var secondaryIndexTables = []string{
	TableIrCodes,
	TableIrOffsets,
	TableIrIndices,
	TableIrNamePool,
	TableIrSources,
}

func isSQLiteDB(db *gorm.DB) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	switch strings.ToLower(db.Dialect().GetName()) {
	case "sqlite", "sqlite3":
		return true
	default:
		return false
	}
}

func quoteSQLIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// SuspendSecondaryIndexes drops non-unique indexes on the IR tables and
// returns the CREATE statements needed to put them back. SQLite only; other
// dialects are a no-op. Unique indexes are not dropped.
func SuspendSecondaryIndexes(db *gorm.DB) ([]string, error) {
	if !isSQLiteDB(db) || db.DB() == nil {
		return nil, nil
	}
	ctx := context.Background()
	conn, err := db.DB().Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	placeholders := make([]string, len(secondaryIndexTables))
	args := make([]any, len(secondaryIndexTables))
	for i, table := range secondaryIndexTables {
		placeholders[i] = "?"
		args[i] = table
	}
	query := `SELECT name, sql FROM sqlite_master WHERE type='index' AND sql IS NOT NULL AND tbl_name IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type indexDDL struct{ name, sql string }
	var indexes []indexDDL
	for rows.Next() {
		var item indexDDL
		if err := rows.Scan(&item.name, &item.sql); err != nil {
			rows.Close()
			return nil, err
		}
		if item.name == "" || item.sql == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(item.sql), "UNIQUE") {
			continue
		}
		indexes = append(indexes, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	ddls := make([]string, 0, len(indexes))
	for _, item := range indexes {
		if _, err := conn.ExecContext(ctx, "DROP INDEX IF EXISTS "+quoteSQLIdent(item.name)); err != nil {
			return ddls, fmt.Errorf("drop index %s: %w", item.name, err)
		}
		ddls = append(ddls, item.sql)
	}
	if len(ddls) > 0 {
		log.Infof("suspended %d secondary IR indexes for the compile write", len(ddls))
	}
	return ddls, nil
}

// RestoreSecondaryIndexes replays CREATE statements captured by
// SuspendSecondaryIndexes. The sorter spills to disk so a multi-gigabyte
// index build does not have to fit in the SQLite page cache.
func RestoreSecondaryIndexes(db *gorm.DB, ddls []string) error {
	if len(ddls) == 0 || !isSQLiteDB(db) || db.DB() == nil {
		return nil
	}
	ctx := context.Background()
	conn, err := db.DB().Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	// Sorting an index build is small next to the database it indexes: the
	// widest single index here is a few hundred megabytes. Spilling it to a
	// temp file costs minutes of extra I/O. Keep it in memory when the host
	// has room, and fall back to the file sorter when it does not.
	tempStore := "FILE"
	if memoryHeadroomForIndexSort() {
		tempStore = "MEMORY"
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA temp_store="+tempStore); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "PRAGMA temp_store=MEMORY")
	log.Infof("restoring %d secondary IR indexes with temp_store=%s", len(ddls), tempStore)

	started := time.Now()
	for _, ddl := range ddls {
		if strings.TrimSpace(ddl) == "" {
			continue
		}
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("restore index %q: %w", ddl, err)
		}
	}
	log.Infof("restored %d secondary IR indexes in %s", len(ddls), time.Since(started))
	return nil
}

// CheckpointSQLiteWAL folds the WAL into the main database once. SSA turns
// automatic checkpoints off so a multi-gigabyte compile does not copy the
// database on every commit; this is the single fold before queries.
func CheckpointSQLiteWAL(db *gorm.DB) error {
	if !isSQLiteDB(db) || db.DB() == nil {
		return nil
	}
	started := time.Now()
	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		return err
	}
	log.Infof("sqlite wal checkpoint in %s", time.Since(started))
	return nil
}
