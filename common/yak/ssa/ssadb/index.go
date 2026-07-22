package ssadb

import (
	"fmt"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

// irIndexBatchChunk bounds the rows per CreateInBatches call so the bind-parameter
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
		if r := db.CreateInBatches(clean, irIndexBatchChunk); r.Error != nil {
			return r.Error
		}
		return nil
	})
	if err != nil {
		fmt.Printf("SaveIrIndexBatch failed: %v\n", err)
	}
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
