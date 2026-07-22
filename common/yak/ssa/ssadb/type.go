package ssadb

import (
	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/utils"
)

// irTypeBatchChunk bounds rows per CreateInBatches call under SQLite's ~999
// host-parameter limit: 150 rows * 5 cols = 750.
const irTypeBatchChunk = 150

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

// SaveIrTypeBatch issues a chunked batched UPSERT for a batch of types: per
// chunk it DELETEs the (program_name, type_id) rows that this batch is about to
// write, then bulk-INSERTs them. This preserves the idempotent-update
// semantics of the old per-row UpsertIrType (a later flush of the same type_id
// overwrites the row with the merged value — see TestTypeFlushUpsertsExisting
// TypeRows) while replacing N select-then-insert round-trips with one DELETE +
// one multi-row INSERT. ir_types has no UNIQUE constraint (only a non-unique
// composite index idx_ir_types_program_type), so ON CONFLICT is unavailable;
// the delete-then-insert inside one transaction is the batched equivalent.
// It still populates the in-process GetIrTypeCache so resident lookups hit.
func SaveIrTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	clean := make([]*IrType, 0, len(items))
	for _, it := range items {
		if it != nil {
			clean = append(clean, it)
		}
	}
	for start := 0; start < len(clean); start += irTypeBatchChunk {
		end := start + irTypeBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		if err := bulkUpsertIrType(db, clean[start:end]); err != nil {
			return err
		}
	}
	// keep the in-process type cache warm (same side effect as UpsertIrType)
	for _, it := range clean {
		if cache := GetIrTypeCache(it.ProgramName); cache != nil {
			cache.Set(int64(it.TypeId), it)
		}
	}
	return nil
}

// bulkUpsertIrType deletes the batch's (program_name, type_id) rows (chunked
// at 999 to respect SQLite's host-parameter limit) then bulk-inserts them via
// CreateInBatches. Must run inside the caller's transaction so the delete +
// insert are atomic.
func bulkUpsertIrType(db *gorm.DB, items []*IrType) error {
	if len(items) == 0 {
		return nil
	}
	// collect distinct (program_name, type_id) pairs to delete. type_id is the
	// logical key within a program; collect ids per program to stay safe if a
	// batch ever spans programs (it does not today, but be correct).
	progTypeIDs := make(map[string][]interface{}, 1)
	for _, it := range items {
		progTypeIDs[it.ProgramName] = append(progTypeIDs[it.ProgramName], it.TypeId)
	}
	for prog, ids := range progTypeIDs {
		for i := 0; i < len(ids); i += 999 {
			end := i + 999
			if end > len(ids) {
				end = len(ids)
			}
			if err := db.Where("program_name = ? AND type_id IN (?)", prog, ids[i:end]).
				Unscoped().Delete(&IrType{}).Error; err != nil {
				return err
			}
		}
	}

	if r := db.CreateInBatches(items, irTypeBatchChunk); r.Error != nil {
		return r.Error
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
