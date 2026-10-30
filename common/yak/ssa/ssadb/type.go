package ssadb

import (
	"github.com/yaklang/gorm"

	"github.com/yaklang/yaklang/common/utils"
)

// irTypeBatchChunk bounds rows per SaveInBatches chunk.
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

func (t *IrType) SetId(id int64) {
	t.TypeId = uint64(id)
}

func (t *IrType) GetIdInt64() int64 {
	return int64(t.TypeId)
}

func (t *IrType) CalcHash(ex ...string) string {
	return utils.CalcSha1(t.ProgramName, t.Kind, t.String, t.ExtraInformation, ex)
}

func UpsertIrType(db *gorm.DB, ir *IrType) error {
	if db == nil || ir == nil {
		return nil
	}
	if err := saveIrTypeRowsDeduped(db, []*IrType{ir}); err != nil {
		return err
	}
	if cache := GetIrTypeCache(ir.ProgramName); cache != nil {
		cache.Set(int64(ir.TypeId), ir)
	}
	return nil
}

// SaveIrTypeBatch stores each body once and upserts one narrow ID row for
// every TypeId. The in-process GetIrTypeCache is warmed after a successful
// write. A caller that owns a transaction also owns commit/rollback.
func SaveIrTypeBatch(db *gorm.DB, items []*IrType) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	if err := saveIrTypeRowsDeduped(db, items); err != nil {
		return err
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
	ir, err := getIrTypeByID(db, progName, id)
	if err != nil {
		// A real read error must not be reported as a missing type; the
		// caller would cache a nil and emit a different IR for this id.
		log.Warnf("read IR type %s/%d: %v", progName, id, err)
		return nil
	}
	return ir
}
