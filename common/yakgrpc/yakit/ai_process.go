package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func CreateAIProcess(db *gorm.DB, process *schema.AiProcess) error {
	db = db.Model(process)
	if db := db.Create(process); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetAIProcessByID(db *gorm.DB, procesID string) (*schema.AiProcess, error) {
	var process schema.AiProcess
	if db = db.Model(&schema.AiProcess{}).Where("process_id = ?", procesID).Preload("Events").First(&process); db.Error != nil {
		return nil, db.Error
	}
	return &process, nil
}
