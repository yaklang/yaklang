package ssadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/yaklang/gorm"
)

// irTypeBatchChunk bounds rows per SaveInBatches chunk.
const irTypeBatchChunk = 1000

// IrType stores one type body per type id.
//
// A program holds one row per distinct type content: typeStore interns equal
// types before it allocates an id, so duplicate ids never reach this table.
// That is what removed the id-to-payload indirection of the previous schema
// without keeping a mapping table.
type IrType struct {
	gorm.Model
	TypeId           uint64 `json:"type_id" gorm:"not null"`
	Kind             int    `json:"kind"`
	ProgramName      string `json:"program_name" gorm:"not null"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
}

func (*IrType) TableName() string {
	return TableIrTypes
}

func (t *IrType) SetId(id int64) {
	t.TypeId = uint64(id)
}

func (t *IrType) GetIdInt64() int64 {
	return int64(t.TypeId)
}

func EmptyIrType(progName string, id uint64) *IrType {
	return &IrType{
		ProgramName: progName,
		TypeId:      id,
	}
}

// SaveIrTypeBatch writes types as in-place upserts on (program_name, type_id).
// ux_ir_types_program_type is created with the schema, so the row id stays
// stable across flushes. SaveInBatches skips nil elements. The in-process
// GetIrTypeCache is warmed after a successful write. GORM selects the native
// upsert or DM MERGE and owns a transaction unless a caller supplied one.
// With a caller-owned transaction, the caller also owns commit/rollback and
// must discard the program cache if it rolls back after this function succeeds.
func SaveIrTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if err := db.New().SaveInBatches(items, irTypeBatchChunk, "program_name", "type_id").Error; err != nil {
		return fmt.Errorf("upsert IR type: %w", err)
	}
	// keep the in-process type cache warm
	for _, it := range items {
		if it == nil {
			continue
		}
		if cache := GetIrTypeCache(it.ProgramName); cache != nil {
			cache.Set(int64(it.TypeId), it)
		}
	}
	return nil
}

// GetIrTypeItemById returns one stored type body, or nil when the id is absent.
func GetIrTypeItemById(db *gorm.DB, progName string, id int64) *IrType {
	if db == nil || id < 0 {
		return nil
	}
	// Bind through database/sql. GORM's Raw query on a handle that already
	// carries model scopes rewrites FROM into "FROM <model>, <query>", which is
	// a syntax error; that turned every type read into nil and once cost ARTEX
	// 97 reflected-XSS risks.
	ctx, cancel := nativeQueryContext(context.Background())
	defer cancel()
	query := bindSQLPlaceholdersDB(db,
		`SELECT kind, "string", extra_information FROM "`+TableIrTypes+`"`+
			` WHERE program_name = ? AND type_id = ? AND deleted_at IS NULL`)
	var kind int
	var text, extra string
	err := nativeQueryRowContext(db, ctx, query, progName, id).Scan(&kind, &text, &extra)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		// A real read error must not be reported as a missing type; the caller
		// would cache a nil and emit a different IR for this id.
		log.Warnf("read IR type %s/%d: %v", progName, id, err)
		return nil
	}
	return &IrType{
		TypeId:           uint64(id),
		ProgramName:      progName,
		Kind:             kind,
		String:           text,
		ExtraInformation: extra,
	}
}
