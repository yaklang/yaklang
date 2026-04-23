package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func UsefulRuntimeId(db *gorm.DB, runtimeId string) (bool, error) {
	if db == nil {
		return false, utils.Errorf("database is nil")
	}
	if runtimeId == "" {
		return false, utils.Errorf("runtime id is empty")
	}

	useful, err := hasRuntimeIdInModel(db, &schema.Risk{}, runtimeId)
	if err != nil {
		return false, err
	}
	if useful {
		return true, nil
	}

	useful, err = hasRuntimeIdInModel(db, &schema.HTTPFlow{}, runtimeId)
	if err != nil {
		return false, err
	}
	return useful, nil
}

func hasRuntimeIdInModel(db *gorm.DB, model interface{}, runtimeId string) (bool, error) {
	rows, err := db.Model(model).
		Select("id").
		Where("runtime_id = ?", runtimeId).
		Limit(1).
		Rows()
	if err != nil {
		return false, err
	}
	defer rows.Close()

	return rows.Next(), nil
}
