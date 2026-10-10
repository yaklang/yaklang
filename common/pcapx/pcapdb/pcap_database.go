package pcapdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mattn/go-sqlite3"
	"github.com/yaklang/gorm"
)

const pcapSQLiteDriver = "pcapdb-sqlite3"

func init() {
	// Apply connection-local flags on every physical connection, including a
	// replacement after the pool discards a connection. On macOS fullfsync asks
	// SQLite to flush the drive cache as well as the OS cache; elsewhere it is
	// harmless. The profile database keeps its own driver and settings.
	sql.Register(pcapSQLiteDriver, &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
		_, err := conn.Exec("PRAGMA fullfsync=ON", nil)
		return err
	}})
}

// Traffic databases own their GORM handles and do not inherit the profile DB's
// durability settings. Read-only connections cannot create missing databases.
func openIndex(path string, write, create bool) (*gorm.DB, error) {
	query := url.Values{"_busy_timeout": {"10000"}, "_foreign_keys": {"on"}, "_cache_size": {"-8192"}}
	query.Set("mode", "ro")
	if write {
		query.Set("mode", "rw")
		if create {
			query.Set("mode", "rwc")
		}
		query.Set("_journal_mode", "WAL")
		query.Set("_synchronous", "FULL")
	} else {
		query.Set("_query_only", "true")
		query.Set("_cache_size", "-2048")
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query.Encode()}
	db, err := gorm.Open("sqlite3", pcapSQLiteDriver, u.String())
	if err != nil {
		return nil, err
	}
	db.LogMode(false)
	if write {
		db.DB().SetMaxOpenConns(1)
		db.DB().SetMaxIdleConns(1)
	} else {
		db.DB().SetMaxOpenConns(4)
		db.DB().SetMaxIdleConns(2)
	}
	if create {
		if err = os.Chmod(path, 0600); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func createIndex(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata) error {
	tx := db.BeginTx(ctx, nil)
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	scoped := indexWithContext(ctx, tx)
	if err := migratePCAPDatabase(scoped); err != nil {
		return err
	}
	if err := scoped.Exec("PRAGMA user_version=" + strconv.Itoa(SchemaVersion)).Error; err != nil {
		return err
	}
	if err := writeManifest(ctx, scoped, meta); err != nil {
		return err
	}
	return tx.Commit().Error
}

func writeManifest(ctx context.Context, db *gorm.DB, meta *PCAPFileDBMetadata) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	record := &PCAPDatasetInfo{Model: gorm.Model{ID: 1}, Metadata: string(raw)}
	return indexWithContext(ctx, db).OnConflictDoUpdate("id", "metadata").Create(record).Error
}

func readManifest(ctx context.Context, db *gorm.DB, id string) (*PCAPFileDBMetadata, error) {
	scoped := indexWithContext(ctx, db)
	var version int
	if err := scoped.Raw("PRAGMA user_version").Row().Scan(&version); err != nil {
		return nil, err
	}
	if version != SchemaVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedSchema, version)
	}
	var record PCAPDatasetInfo
	if err := scoped.Where("id = ?", 1).First(&record).Error; err != nil {
		return nil, err
	}
	var meta PCAPFileDBMetadata
	if err := json.Unmarshal([]byte(record.Metadata), &meta); err != nil {
		return nil, err
	}
	if meta.DatasetID != id || meta.SchemaVersion != version || meta.DatabaseType != "sqlite" || !knownState(meta.State) {
		return nil, fmt.Errorf("pcapdb: inconsistent dataset manifest")
	}
	if meta.SourceSize < 0 || meta.PacketCount < 0 || meta.ProtocolCount < 0 || meta.ProtocolDataSize < 0 || meta.BytesIndexed < 0 || meta.BytesIndexed > meta.SourceSize || meta.CaptureRecordCount < 0 || meta.SessionCount < 0 || meta.StreamCount < 0 || meta.StreamChunkCount < 0 || meta.StreamDataSize < 0 {
		return nil, fmt.Errorf("pcapdb: invalid checkpoint counters")
	}
	if meta.State == StateReady && (len(meta.FullSHA256) != 64 || meta.BytesIndexed != meta.SourceSize || (meta.Format != "pcap" && meta.Format != "pcapng")) {
		return nil, fmt.Errorf("pcapdb: incomplete ready checkpoint")
	}
	if meta.State == StateReady {
		if meta.CaptureRecordCount == 0 || (meta.ProtocolsIndexed && len(meta.ProtocolDataSHA256) != 64) || (!meta.ProtocolsIndexed && (meta.ProtocolCount != 0 || meta.ProtocolDataSize != 0)) || (meta.StreamsIndexed && (meta.StreamCount != 2*meta.SessionCount || len(meta.StreamDataSHA256) != 64)) || (!meta.StreamsIndexed && (meta.SessionCount != 0 || meta.StreamChunkCount != 0 || meta.StreamDataSize != 0)) {
			return nil, fmt.Errorf("pcapdb: incomplete analysis checkpoint")
		}
	}
	var tables int
	names := []string{"dataset_info", "capture_interfaces", "packets", "protocol_messages", "message_packets", "capture_records", "sessions", "streams", "stream_chunks", "session_packets", "stream_chunk_packets"}
	if err := scoped.Table("sqlite_schema").Where("type = ? AND name IN (?)", "table", names).Count(&tables).Error; err != nil {
		return nil, err
	}
	if tables != len(names) {
		return nil, fmt.Errorf("pcapdb: index tables are missing")
	}
	return &meta, nil
}

// This repository's GORM v1 has BeginTx but no WithContext. Bind its SQLCommon
// interface to ExecContext/QueryContext so cancellation also interrupts a long
// JSON search. Models, CRUD, migration and transaction ownership stay in GORM.
type indexSQLConnection interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type indexConnection struct {
	ctx        context.Context
	connection indexSQLConnection
}

func indexWithContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	var connection indexConnection
	if bound, ok := db.CommonDB().(indexConnection); ok {
		connection = bound
	} else {
		connection.connection = db.CommonDB().(indexSQLConnection)
	}
	connection.ctx = ctx
	// SQLCommon is already open; gorm.Open cannot perform an I/O operation here.
	scoped, _ := gorm.Open("sqlite3", connection)
	scoped.LogMode(false)
	return scoped
}

func (c indexConnection) Exec(query string, args ...interface{}) (sql.Result, error) {
	return c.connection.ExecContext(c.ctx, query, args...)
}
func (c indexConnection) Prepare(query string) (*sql.Stmt, error) {
	return c.connection.PrepareContext(c.ctx, query)
}
func (c indexConnection) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return c.connection.QueryContext(c.ctx, query, args...)
}
func (c indexConnection) QueryRow(query string, args ...interface{}) *sql.Row {
	return c.connection.QueryRowContext(c.ctx, query, args...)
}

// Run only on the serialized writer, after publication or after all of this
// manager's readers close. An active external snapshot may defer WAL cleanup.
func checkpointIndex(ctx context.Context, db *gorm.DB) error {
	var busy, frames, copied int
	return indexWithContext(ctx, db).Raw("PRAGMA wal_checkpoint(PASSIVE)").Row().Scan(&busy, &frames, &copied)
}
