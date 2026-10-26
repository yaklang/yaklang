package ssadb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/gorm"
)

// Large projects persist millions of type rows that share a handful of
// payloads: Hue writes 3.47M ir_types rows for 106K distinct payloads. The
// payload is what makes a row wide, so the identity rows live in their own
// narrow table and each payload is stored once.
//
//	ir_type_ids_v1     (program_name, type_id, type_hash)   -- one row per type id
//	ir_type_payloads_v1(program_name, type_hash, kind, string, extra)
//
// A database written this way has no ir_types rows. Readers resolve the id
// through the join below and fall back to ir_types for databases compiled by
// older binaries; a database is never rewritten just to change layout.
const (
	TableIrTypeIDs      = "ir_type_ids_v1"
	TableIrTypePayloads = "ir_type_payloads_v1"
)

// IrTypeID maps one type id to its shared payload.
type IrTypeID struct {
	gorm.Model
	ProgramName string `json:"program_name" gorm:"not null"`
	TypeID      uint64 `json:"type_id" gorm:"not null"`
	TypeHash    string `json:"type_hash" gorm:"type:varchar(64);not null"`
}

func (*IrTypeID) TableName() string { return TableIrTypeIDs }

// IrTypePayload is the deduplicated type body.
type IrTypePayload struct {
	gorm.Model
	ProgramName      string `json:"program_name" gorm:"not null"`
	TypeHash         string `json:"type_hash" gorm:"type:varchar(64);not null"`
	Kind             int    `json:"kind"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
}

func (*IrTypePayload) TableName() string { return TableIrTypePayloads }

// typePayloadHash identifies a payload. It must cover every column the payload
// table stores, so two types hash equal exactly when their bodies are equal.
func typePayloadHash(kind int, text, extra string) string {
	payload := make([]byte, 0, len(text)+len(extra)+24)
	var scratch [8]byte
	binary.LittleEndian.PutUint64(scratch[:], uint64(int64(kind)))
	payload = append(payload, scratch[:]...)
	binary.LittleEndian.PutUint64(scratch[:], uint64(len(text)))
	payload = append(payload, scratch[:]...)
	payload = append(payload, text...)
	binary.LittleEndian.PutUint64(scratch[:], uint64(len(extra)))
	payload = append(payload, scratch[:]...)
	payload = append(payload, extra...)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// dedupeReady caches per connection pool whether the dedupe tables exist, so a
// legacy database does not pay a failed query on every type read. The tables
// are part of the SSA schema, so a database created by this binary has them
// before the first write; the cache only skips repeated sqlite_master probes.
var (
	dedupeMu    sync.Mutex
	dedupeReady = map[*sql.DB]bool{}
)

// IrTypeDedupeAvailable reports whether this pool stores types in the dedupe
// layout. It never migrates: an in-transaction write path must not run DDL.
func IrTypeDedupeAvailable(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	// A transaction handle cannot expose its *sql.DB for caching, but it can
	// verify the schema with direct zero-row queries. Use database/sql here:
	// GORM's Raw.Rows can add model scopes left on a cloned transaction handle.
	if tx, inTx := db.CommonDB().(*sql.Tx); inTx {
		for _, table := range []string{TableIrTypeIDs, TableIrTypePayloads} {
			rows, err := tx.QueryContext(context.Background(), "SELECT 1 FROM "+table+" LIMIT 0")
			if err != nil {
				return false
			}
			_ = rows.Close()
		}
		return true
	}
	if db.DB() == nil {
		return false
	}
	pool := db.DB()
	dedupeMu.Lock()
	defer dedupeMu.Unlock()
	if ready, ok := dedupeReady[pool]; ok {
		return ready
	}
	ready := db.HasTable(TableIrTypeIDs) && db.HasTable(TableIrTypePayloads)
	dedupeReady[pool] = ready
	return ready
}

// resetIrTypeDedupeAvailability drops the cached layout decision. Tests that
// build the schema after a first probe use it.
func resetIrTypeDedupeAvailability() {
	dedupeMu.Lock()
	defer dedupeMu.Unlock()
	dedupeReady = map[*sql.DB]bool{}
}

func invalidateIrTypeDedupeAvailability(db *gorm.DB) {
	if db == nil {
		return
	}
	pool := db.DB()
	if pool == nil {
		return
	}
	dedupeMu.Lock()
	delete(dedupeReady, pool)
	dedupeMu.Unlock()
}

// ensureIrTypeDedupeIndexes installs the two conflict targets the batch writes
// rely on. Both are UNIQUE, so a replayed flush cannot double-write a payload
// or move a type id to a different payload.
func ensureIrTypeDedupeIndexes(db *gorm.DB) error {
	if db == nil || db.Dialect() == nil {
		return errors.New("no dialect")
	}
	queries := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_type_ids_program_type" ON "` + TableIrTypeIDs + `" ("program_name", "type_id");`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "ux_ir_type_payloads_program_hash" ON "` + TableIrTypePayloads + `" ("program_name", "type_hash");`,
	}
	for _, query := range queries {
		if err := db.Exec(query).Error; err != nil {
			return err
		}
	}
	return nil
}

// saveIrTypeRowsDeduped writes the payloads that are new and every id row.
// Payload equality is a hash over the body, so the statement re-sent for a
// revisited type conflicts and does nothing instead of rewriting its text.
func saveIrTypeRowsDeduped(db *gorm.DB, items []*IrType) error {
	payloads := make([]*IrTypePayload, 0, len(items))
	seenPayload := make(map[typePayloadKey]*IrTypePayload, len(items))
	idsByKey := make(map[typeIDKey]*IrTypeID, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		hash := typePayloadHash(item.Kind, item.String, item.ExtraInformation)
		key := typePayloadKey{program: item.ProgramName, hash: hash}
		if payload, ok := seenPayload[key]; ok {
			if payload.Kind != item.Kind || payload.String != item.String || payload.ExtraInformation != item.ExtraInformation {
				return fmt.Errorf("SHA-256 collision in IR type batch for program %q hash %s", item.ProgramName, hash)
			}
		} else {
			payload := &IrTypePayload{
				ProgramName:      item.ProgramName,
				TypeHash:         hash,
				Kind:             item.Kind,
				String:           item.String,
				ExtraInformation: item.ExtraInformation,
			}
			seenPayload[key] = payload
			payloads = append(payloads, payload)
		}
		// A batch may contain repeated IDs. Keep the last body, matching SQLite's
		// historical ordered-upsert behavior and avoiding PostgreSQL's duplicate
		// conflict-target error for one multi-row statement.
		idsByKey[typeIDKey{program: item.ProgramName, id: item.TypeId}] = &IrTypeID{
			ProgramName: item.ProgramName,
			TypeID:      item.TypeId,
			TypeHash:    hash,
		}
	}
	if len(idsByKey) == 0 {
		return nil
	}
	ids := make([]*IrTypeID, 0, len(idsByKey))
	for _, item := range idsByKey {
		ids = append(ids, item)
	}
	if err := insertTypePayloads(db, payloads); err != nil {
		return err
	}
	started := time.Now()
	result := db.New().SaveInBatches(ids, irTypeBatchChunk, "program_name", "type_id")
	RecordDBOp(DBOpUpdate, time.Since(started), result.Error != nil)
	if err := result.Error; err != nil {
		return fmt.Errorf("upsert IR type ids: %w", err)
	}
	return nil
}

type typeIDKey struct {
	program string
	id      uint64
}

type typePayloadKey struct {
	program string
	hash    string
}

// insertTypePayloads stores payload bodies once. SQLite, PostgreSQL and MySQL
// can say "ignore the duplicate" in the INSERT itself, which costs one
// statement per chunk and never rewrites a stored body.
func insertTypePayloads(db *gorm.DB, payloads []*IrTypePayload) error {
	if len(payloads) == 0 {
		return nil
	}
	if option, ok := typePayloadInsertOption(db); ok {
		started := time.Now()
		result := db.CreateInBatchesOnConflict(payloads, irTypeBatchChunk, option)
		RecordDBOp(DBOpCreate, time.Since(started), result.Error != nil)
		if err := result.Error; err != nil {
			return fmt.Errorf("insert IR type payloads: %w", err)
		}
		return nil
	}
	missing, err := missingTypePayloads(db, payloads)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	started := time.Now()
	result := db.CreateInBatches(missing, irTypeBatchChunk)
	RecordDBOp(DBOpCreate, time.Since(started), result.Error != nil)
	if err := result.Error; err != nil {
		return fmt.Errorf("insert IR type payloads: %w", err)
	}
	return nil
}

// typePayloadInsertOption returns the conflict clause for dialects that accept
// one inline. Dialects without it fall back to a lookup before the insert.
func typePayloadInsertOption(db *gorm.DB) (string, bool) {
	if db == nil || db.Dialect() == nil {
		return "", false
	}
	switch db.Dialect().GetName() {
	case "sqlite3", "sqlite", "postgres", "postgresql":
		return "ON CONFLICT (program_name, type_hash) DO NOTHING", true
	default:
		return "", false
	}
}

// missingTypePayloads returns the payloads whose hash is not stored yet. The
// lookup is chunked so a large first flush stays under the bind-parameter cap.
func missingTypePayloads(db *gorm.DB, payloads []*IrTypePayload) ([]*IrTypePayload, error) {
	byProgram := make(map[string][]*IrTypePayload)
	for _, payload := range payloads {
		byProgram[payload.ProgramName] = append(byProgram[payload.ProgramName], payload)
	}
	var missing []*IrTypePayload
	for program, group := range byProgram {
		for start := 0; start < len(group); start += irTypeBatchChunk {
			end := start + irTypeBatchChunk
			if end > len(group) {
				end = len(group)
			}
			hashes := make([]string, 0, end-start)
			for _, payload := range group[start:end] {
				hashes = append(hashes, payload.TypeHash)
			}
			var stored []IrTypePayload
			if err := db.Select("type_hash, kind, string, extra_information").
				Model(&IrTypePayload{}).
				Where("program_name = ? AND type_hash IN (?)", program, hashes).
				Find(&stored).Error; err != nil {
				return nil, fmt.Errorf("look up IR type payloads: %w", err)
			}
			seen := make(map[string]IrTypePayload, len(stored))
			for _, row := range stored {
				seen[row.TypeHash] = row
			}
			for _, payload := range group[start:end] {
				if got, ok := seen[payload.TypeHash]; ok {
					if got.Kind != payload.Kind || got.String != payload.String ||
						got.ExtraInformation != payload.ExtraInformation {
						return nil, fmt.Errorf(
							"SHA-256 collision in IR type payloads for program %q hash %s",
							program, payload.TypeHash)
					}
					continue
				}
				missing = append(missing, payload)
			}
		}
	}
	return missing, nil
}

// getIrTypeFromDedupe resolves one type id. It returns (nil, nil) when the id
// is not stored in the dedupe tables, which is the signal to read ir_types.
func getIrTypeFromDedupe(db *gorm.DB, progName string, id int64) (*IrType, error) {
	if db == nil || id < 0 {
		return nil, nil
	}
	// A legacy database must not pay a failing JOIN on every type read.
	if !IrTypeDedupeAvailable(db) {
		return nil, nil
	}
	var kind int
	var text, extra string
	err := db.Raw(
		`SELECT p.kind AS kind, p.string AS string, p.extra_information AS extra_information`+
			` FROM `+TableIrTypeIDs+` AS t`+
			` JOIN `+TableIrTypePayloads+` AS p`+
			` ON p.program_name = t.program_name AND p.type_hash = t.type_hash`+
			` WHERE t.program_name = ? AND t.type_id = ? AND t.deleted_at IS NULL AND p.deleted_at IS NULL`,
		progName, id).Row().Scan(&kind, &text, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		// Keep the cached layout decision: a transient query failure must not
		// silently redirect later writes to ir_types, which would split one
		// program across two layouts.
		return nil, err
	}
	return &IrType{
		TypeId:           uint64(id),
		ProgramName:      progName,
		Kind:             kind,
		String:           text,
		ExtraInformation: extra,
	}, nil
}

// HasIrTypeDedupeRows reports whether a program uses the dedupe layout. Callers
// that need a count of persisted types use it instead of counting ir_types.
func HasIrTypeDedupeRows(db *gorm.DB, program string) bool {
	if db == nil || !IrTypeDedupeAvailable(db) {
		return false
	}
	var one int
	err := db.Raw(`SELECT 1 FROM `+TableIrTypeIDs+` WHERE program_name = ? LIMIT 1`, program).Row().Scan(&one)
	return err == nil
}
