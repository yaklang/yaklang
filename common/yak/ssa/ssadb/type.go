package ssadb

import (
	"fmt"

	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/utils"
)

// irTypeBatchChunk bounds rows per SaveInBatches chunk. The dedupe layout uses
// it for narrow type-ID rows; legacy databases use it for full IrType rows.
const irTypeBatchChunk = 1000

type IrType struct {
	gorm.Model
	TypeId           uint64 `json:"type_id"`
	Kind             int    `json:"kind"`
	ProgramName      string `json:"program_name"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
	// Hash             string `json:"hash" gorm:"unique_index"`
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

func (t *IrType) CalcHash(ex ...string) string {
	return utils.CalcSha1(t.ProgramName, t.Kind, t.String, t.ExtraInformation, ex)
}

func (ir *IrType) Save(db *gorm.DB) error {
	return db.Save(ir).Error
}

func UpsertIrType(db *gorm.DB, ir *IrType) error {
	if db == nil || ir == nil {
		return nil
	}
	if IrTypeDedupeAvailable(db) {
		if err := saveIrTypeRowsDeduped(db, []*IrType{ir}); err != nil {
			return err
		}
		if cache := GetIrTypeCache(ir.ProgramName); cache != nil {
			cache.Set(int64(ir.TypeId), ir)
		}
		return nil
	}
	record := &IrType{
		ProgramName: ir.ProgramName,
		TypeId:      ir.TypeId,
	}
	if err := db.Where("program_name = ? AND type_id = ?", ir.ProgramName, ir.TypeId).
		Assign(ir).
		FirstOrCreate(record).Error; err != nil {
		return err
	}
	if cache := GetIrTypeCache(ir.ProgramName); cache != nil {
		cache.Set(int64(ir.TypeId), ir)
	}
	return nil
}

// SaveIrTypeBatch persists a batch in the dedupe layout when the additive
// tables exist. It stores each body once and upserts one narrow ID row for
// every TypeId. Databases without those tables retain the legacy ir_types
// upsert path. New and legacy readers are supported in the same process.
// The in-process GetIrTypeCache is warmed after a successful write. A caller
// that owns a transaction also owns commit/rollback.
func SaveIrTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if IrTypeDedupeAvailable(db) {
		if err := saveIrTypeRowsDeduped(db, items); err != nil {
			return err
		}
	} else {
		if err := db.New().SaveInBatches(items, irTypeBatchChunk, "program_name", "type_id").Error; err != nil {
			return fmt.Errorf("upsert IR type: %w", err)
		}
	}
	// keep the in-process type cache warm (same side effect as UpsertIrType)
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

func EmptyIrType(progName string, id uint64) *IrType {
	return &IrType{
		ProgramName: progName,
		TypeId:      id,
	}
}

func GetIrTypeItemById(db *gorm.DB, progName string, id int64) *IrType {
	if id < 0 {
		return nil
	}
	// Deduped databases have no ir_types row for this id; the join returns the
	// shared payload. A database compiled by an older binary answers nil here
	// and falls through to the ir_types read below.
	if deduped, err := getIrTypeFromDedupe(db, progName, id); err == nil && deduped != nil {
		return deduped
	} else if err != nil {
		// A real read error must not be reported as a missing type; the
		// caller would cache a nil and emit a different IR for this id.
		log.Warnf("read deduped IR type %s/%d: %v", progName, id, err)
		return nil
	}
	// Native-SQL fast path (see GetIrCodeItemById comment). Same result as the
	// GORM First() path; falls back to GORM on any native error.
	native, err := nativeGetIrTypeItemByIdErr(db, progName, id)
	if native != nil {
		return native
	}
	if nativeContextErr(err) {
		return nil
	}
	// check cache
	ir := &IrType{}
	// db = db.Debug()
	if db := db.Model(&IrType{}).
		Where("type_id = ?", id).
		Where("program_name = ?", progName).
		First(ir); db.Error != nil {
		return nil
	}
	return ir
}

func DeleteIrType(db *gorm.DB, id []int64) error {
	// log.Errorf("DeleteIrType: %d", len(id))
	if len(id) == 0 {
		return utils.Errorf("delete type from database id is empty")
	}
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		// Split below SQLite's traditional bind-variable limit.
		for i := 0; i < len(id); i += 999 {
			end := i + 999
			if end > len(id) {
				end = len(id)
			}
			chunk := id[i:end]
			if err := tx.Where("id IN (?)", chunk).Unscoped().Delete(&IrType{}).Error; err != nil {
				return err
			}
		}
		// Dedupe tables are keyed by type_id, not by the legacy ir_types
		// primary key, so this call cannot name one of their rows. Drop only
		// payloads that no id row references any more.
		if IrTypeDedupeAvailable(tx) {
			cleanup := tx.Exec(`DELETE FROM ` + TableIrTypePayloads + ` AS p WHERE NOT EXISTS (` +
				`SELECT 1 FROM ` + TableIrTypeIDs + ` AS i WHERE i.program_name = p.program_name ` +
				`AND i.payload_id = p.id)`)
			if cleanup.Error != nil {
				return cleanup.Error
			}
		}
		return nil
	})
}
