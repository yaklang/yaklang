package ssadb

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

// irIndexInsertColumns are the IrIndex application columns written by the
// batched multi-row INSERT below. The gorm.Model fields (id/created_at/
// updated_at/deleted_at) are left to SQLite autoincrement / defaults, matching
// the previous per-row db.Create behavior (which also did not set them).
var irIndexInsertColumns = []string{
	"program_name", "value_id", "variable_id", "class_id", "field_id",
	"scope_name", "owner_value_id", "version_id",
}

// irIndexBatchChunk bounds the rows per multi-row INSERT so the bind-parameter
// count stays under SQLite's ~999 host-parameter limit: 100 rows * 8 cols = 800.
const irIndexBatchChunk = 100

// IrIndex is the database model for index entries (normalized with IDs).
type IrIndex struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index;not null"`
	ValueID     int64  `json:"value_id" gorm:"index;not null"`

	VariableID *int64 `json:"variable_id" gorm:"index"`
	ClassID    *int64 `json:"class_id" gorm:"index"`
	FieldID    *int64 `json:"field_id" gorm:"index"`

	// scope
	ScopeName string `json:"scope_name" gorm:"index"`

	// for object-key-member search
	// owner id + field id -> member
	OwnerValueID *int64 `json:"owner_value_id" gorm:"index"`

	VersionID int64 `json:"version_id" gorm:"index"`
}

func (i *IrIndex) TableName() string {
	return TableIrIndices
}

func CreateIndex(progName string) *IrIndex {
	ret := &IrIndex{
		ProgramName: progName,
	}
	return ret
}

func SaveIrIndex(db *gorm.DB, idx *IrIndex) {
	if idx == nil || db == nil {
		return
	}
	SaveIrIndexBatch(db, []*IrIndex{idx})
}

func SaveIrIndexBatch(db *gorm.DB, items []*IrIndex) {
	if db == nil || len(items) == 0 {
		return
	}
	clean := make([]*IrIndex, 0, len(items))
	for _, it := range items {
		if it != nil {
			clean = append(clean, it)
		}
	}
	if len(clean) == 0 {
		return
	}

	err := diagnostics.TrackLow("Database.SaveIRIndexBatch", func() error {
		for start := 0; start < len(clean); start += irIndexBatchChunk {
			end := start + irIndexBatchChunk
			if end > len(clean) {
				end = len(clean)
			}
			if err := bulkInsertIrIndex(db, clean[start:end]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("SaveIrIndexBatch failed: %v\n", err)
	}
}

// bulkInsertIrIndex issues a single multi-row INSERT for one chunk. There is no
// UNIQUE constraint on ir_indices_v1 (only a non-unique index), and recompile
// deletes the program's rows first (ssadb.DeleteProgramIrCode), so this is a
// pure INSERT — never an upsert — matching the prior per-row db.Create path.
func bulkInsertIrIndex(db *gorm.DB, items []*IrIndex) error {
	if len(items) == 0 {
		return nil
	}
	const cols = 8
	placeholder := "(" + strings.Repeat("?,", cols-1) + "?)"
	values := make([]string, 0, len(items))
	args := make([]interface{}, 0, len(items)*cols)
	for _, it := range items {
		values = append(values, placeholder)
		args = append(args,
			it.ProgramName, it.ValueID, it.VariableID, it.ClassID, it.FieldID,
			it.ScopeName, it.OwnerValueID, it.VersionID,
		)
	}
	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		TableIrIndices,
		strings.Join(irIndexInsertColumns, ","),
		strings.Join(values, ","),
	)
	return db.Exec(sql, args...).Error
}

type IrVariable struct {
	VariableName string
	ValueID      int64
	VersionID    int64
}

func GetScope(programName, scopeName string, cache *NameCache) ([]IrVariable, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if scopeName == "" {
		return []IrVariable{}, nil
	}

	// Select raw data from indices
	type result struct {
		VariableID *int64
		ValueID    int64
		VersionID  int64
	}
	var results []result

	err := db.Model(&IrIndex{}).
		Select("variable_id, value_id, version_id").
		Where("program_name = ? AND scope_name = ? AND deleted_at IS NULL", programName, scopeName).
		Where(`version_id = (
			SELECT max(sub.version_id) FROM ` + TableIrIndices + ` AS sub
			WHERE sub.variable_id = ` + TableIrIndices + `.variable_id
			  AND sub.scope_name = ` + TableIrIndices + `.scope_name
			  AND sub.program_name = ` + TableIrIndices + `.program_name
		)`).Scan(&results).Error

	if err != nil {
		return nil, err
	}

	// Resolve names locally using cache
	ret := make([]IrVariable, 0, len(results))
	for _, res := range results {
		if res.VariableID == nil {
			continue
		}
		name := cache.GetName(*res.VariableID)
		if name == "" {
			continue
		}
		ret = append(ret, IrVariable{
			VariableName: name,
			ValueID:      res.ValueID,
			VersionID:    res.VersionID,
		})
	}
	return ret, nil
}
