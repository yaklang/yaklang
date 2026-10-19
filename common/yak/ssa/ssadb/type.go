package ssadb

import (
	"fmt"

	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/utils"
)

// irTypeBatchChunk bounds rows per CreateInBatches call under SQLite's
// MAX_VARIABLE_NUMBER=250000 ceiling; 1000 rows is safe for the wide IrType
// model and reduces the number of multi-row INSERT statements per flush.
const irTypeBatchChunk = 1000

type IrType struct {
	gorm.Model
	TypeId           uint64 `json:"type_id" gorm:"index"`
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

// SaveIrTypeBatch writes types as in-place upserts on (program_name, type_id).
// ux_ir_types_program_type is created with the schema, so the row id stays
// stable across flushes. SaveInBatches skips nil elements. The in-process
// GetIrTypeCache is warmed either way.
func SaveIrTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if err := db.New().SaveInBatches(items, irTypeBatchChunk, "program_name", "type_id").Error; err != nil {
		return fmt.Errorf("upsert IR type: %w", err)
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
		// split each 999
		for i := 0; i < len(id); i += 999 {
			end := i + 999
			if end > len(id) {
				end = len(id)
			}
			tx.Where("id IN (?)", id[i:end]).Unscoped().Delete(&IrType{})
		}
		return nil
	})
}
