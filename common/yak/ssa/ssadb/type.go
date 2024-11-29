package ssadb

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type IrType struct {
	gorm.Model
	Kind             int    `json:"kind"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
	Hash             string `json:"hash" gorm:"unique_index"`
}

func (t *IrType) CalcHash() string {
	return utils.CalcSha1(t.Kind, t.String, t.ExtraInformation)
}

func SaveType(kind int, str string, extra string) int {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSASaveTypeCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	irType := IrType{
		Kind:             kind,
		String:           str,
		ExtraInformation: extra,
	}
	// TODO: ignore reuse ir-type
	irType.Hash = irType.CalcHash() + uuid.NewString()

	// err := utils.AttemptWithDelayFast(func() error {
	// return utils.GormTransaction(GetDB(), func(tx *gorm.DB) error {
	// if queryDB := tx.Model(&IrType{}).Where("hash = ? ", irType.Hash).First(&irType); queryDB.Error != nil {
	// 	if !queryDB.RecordNotFound() {
	// 		log.Errorf("query error :%s", queryDB.Error)
	// 		return queryDB.Error
	// 	}
	// }
	// if saveDB := tx.Model(&IrType{}).Save(&irType); saveDB.Error != nil {
	// 	log.Errorf("save error :%s", saveDB.Error)
	// 	return saveDB.Error
	// }
	// return nil
	// })
	// })
	err := GetDB().Model(&IrType{}).Save(&irType).Error
	if err != nil {
		log.Errorf("SaveType error: %v", err)
		return -1
	}

	return int(irType.ID)
}

func GetType(id int) (int, string, string, error) {
	if id == -1 {
		return 0, "", "", utils.Errorf("get type from database id is -1")
	}
	db := GetDB()
	irType := &IrType{}
	if db := db.First(irType, id); db.Error != nil {
		return 0, "", "", db.Error
	}
	return irType.Kind, irType.String, irType.ExtraInformation, nil
}
