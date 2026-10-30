package ssadb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/yaklang/gorm"
)

// Large projects persist millions of type rows that share a handful of
// payloads: Hue wrote 3.47M type rows for 106K distinct payloads. The payload
// is what makes a row wide, so the identity rows live in their own narrow
// table and each payload is stored once.
//
//	ir_type_ids     (program_name, type_id, payload_id)  -- one row per type id
//	ir_type_payloads(program_name, type_hash, kind, string, extra)
//
// The payload table is the only place a type body is stored. There is no
// second layout and no migration: a database written by an older binary is
// not read.
const (
	TableIrTypeIDs      = "ir_type_ids"
	TableIrTypePayloads = "ir_type_payloads"
)

// IrTypeID maps one type id to its shared payload. It deliberately carries no
// gorm.Model: three extra columns (id, created_at, updated_at) made each row
// ~185 bytes, wider than the ir_types row it replaces. The payload is
// referenced by its integer primary key, so the row is only the program, the
// type id and the reference.
type IrTypeID struct {
	ProgramName string `json:"program_name" gorm:"not null"`
	TypeID      uint64 `json:"type_id" gorm:"not null"`
	PayloadID   int64  `json:"payload_id" gorm:"not null"`
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
// The id row holds an integer payload reference, so a revisited type only
// rewrites three narrow columns instead of its whole body.
func saveIrTypeRowsDeduped(db *gorm.DB, items []*IrType) error {
	payloads := make([]*IrTypePayload, 0, len(items))
	seenPayload := make(map[typePayloadKey]*IrTypePayload, len(items))
	payloadHashByID := make(map[typeIDKey]string, len(items))
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
		payloadHashByID[typeIDKey{program: item.ProgramName, id: item.TypeId}] = hash
	}
	if len(payloadHashByID) == 0 {
		return nil
	}
	if err := insertTypePayloads(db, payloads); err != nil {
		return err
	}
	payloadIDs, err := lookupTypePayloadIDs(db, payloads)
	if err != nil {
		return err
	}
	ids := make([]*IrTypeID, 0, len(payloadHashByID))
	for key, hash := range payloadHashByID {
		payloadID, ok := payloadIDs[typePayloadKey{program: key.program, hash: hash}]
		if !ok || payloadID <= 0 {
			return fmt.Errorf("IR type payload for program %q hash %s has no row id", key.program, hash)
		}
		ids = append(ids, &IrTypeID{ProgramName: key.program, TypeID: key.id, PayloadID: payloadID})
	}
	started := time.Now()
	result := db.New().SaveInBatches(ids, irTypeBatchChunk, "program_name", "type_id")
	RecordDBOp(DBOpUpdate, time.Since(started), result.Error != nil)
	if err := result.Error; err != nil {
		return fmt.Errorf("upsert IR type ids: %w", err)
	}
	return nil
}

// lookupTypePayloadIDs resolves the primary key of each payload just written or
// already stored. One narrow read per batch replaces the previous per-row
// 64-character hash column.
func lookupTypePayloadIDs(db *gorm.DB, payloads []*IrTypePayload) (map[typePayloadKey]int64, error) {
	out := make(map[typePayloadKey]int64, len(payloads))
	if len(payloads) == 0 {
		return out, nil
	}
	byProgram := make(map[string][]string)
	for _, payload := range payloads {
		byProgram[payload.ProgramName] = append(byProgram[payload.ProgramName], payload.TypeHash)
	}
	for program, hashes := range byProgram {
		for start := 0; start < len(hashes); start += irTypeBatchChunk {
			end := start + irTypeBatchChunk
			if end > len(hashes) {
				end = len(hashes)
			}
			var rows []IrTypePayload
			if err := db.Select("id, type_hash").Model(&IrTypePayload{}).
				Where("program_name = ? AND type_hash IN (?)", program, hashes[start:end]).
				Find(&rows).Error; err != nil {
				return nil, fmt.Errorf("look up IR type payload ids: %w", err)
			}
			for _, row := range rows {
				out[typePayloadKey{program: program, hash: row.TypeHash}] = int64(row.ID)
			}
		}
	}
	return out, nil
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

// getIrTypeByID resolves one type id through the payload join. It returns
// (nil, nil) when the id is not stored.
func getIrTypeByID(db *gorm.DB, progName string, id int64) (*IrType, error) {
	if db == nil || id < 0 {
		return nil, nil
	}
	var kind int
	var text, extra string
	// Bind the query through database/sql. GORM's Raw query on a handle that
	// already carries model scopes rewrites FROM into "FROM <model>, <query>",
	// which is a syntax error and silently turned every type read into nil.
	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT p.kind, p."string", p.extra_information`+
			` FROM `+TableIrTypeIDs+` AS t`+
			` JOIN `+TableIrTypePayloads+` AS p`+
			` ON p.id = t.payload_id AND p.program_name = t.program_name`+
			` WHERE t.program_name = ? AND t.type_id = ? AND p.deleted_at IS NULL`)
	err := nativeQueryRowContext(db, ctx, query, progName, id).Scan(&kind, &text, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
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

// EnsureIrTypeDedupeIndexesForTest installs the type conflict indexes on a bare
// test database that migrated the two models directly.
func EnsureIrTypeDedupeIndexesForTest(db *gorm.DB) error {
	return ensureIrTypeDedupeIndexes(db)
}
