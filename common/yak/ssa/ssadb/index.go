package ssadb

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
)

// IrIndex is the database model for index entries (normalized with IDs).
type IrIndex struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index;not null"`
	ValueID     int64  `json:"value_id" gorm:"index;not null"`

	VariableID *int64 `json:"variable_id" gorm:"index"`
	ClassID    *int64 `json:"class_id" gorm:"index"`
	FieldID    *int64 `json:"field_id" gorm:"index"`

	// scope
	ScopeID *int64 `json:"scope_id" gorm:"index"`

	// for object-key-member search
	// owner id + field id -> member
	OwnerValueID *int64 `json:"owner_value_id" gorm:"index"`

	VersionID int64 `json:"version_id" gorm:"index"`
}

func (i *IrIndex) TableName() string {
	return "ir_indices"
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

	err := diagnostics.TrackLow("Database.SaveIRIndexBatch", func() error {
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := db.Create(item).Error; err != nil {
				return err
			}
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

	scopeID := cache.GetID(programName, scopeName)
	if scopeID == 0 {
		return []IrVariable{}, nil
	}

	// Select raw data from indices
	type result struct {
		VariableID *int64
		ValueID    int64
		VersionID  int64
	}
	var results []result

	err := db.Table("ir_indices").
		Select("variable_id, value_id, version_id").
		Where("program_name = ? AND scope_id = ? AND deleted_at IS NULL", programName, scopeID).
		Where(`version_id = (
			SELECT max(sub.version_id) FROM ir_indices AS sub
			WHERE sub.variable_id = ir_indices.variable_id 
			  AND sub.scope_id = ir_indices.scope_id 
			  AND sub.program_name = ir_indices.program_name
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
		name := cache.GetName(programName, *res.VariableID)
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
