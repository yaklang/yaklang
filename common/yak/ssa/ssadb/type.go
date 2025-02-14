package ssadb

import (
	"github.com/yaklang/yaklang/common/log"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
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
		atomic.AddUint64(&_SSASaveTypeCost, uint64(time.Since(start).Nanoseconds()))
	}()

	irType := IrType{

		Kind:             kind,
		String:           str,
		ExtraInformation: extra,
	}
	irType.Hash = irType.CalcHash()

	db := GetDB()
	/*
		todo: check it why db locked. this is tmp resolve
	*/
	utils.GormTransaction(db, func(tx *gorm.DB) error {
		count := 0
	retry:
		err := tx.Where("hash = ?", irType.Hash).FirstOrCreate(&irType).Error
		if err != nil {
			if count < 5 {
				count++
				goto retry
			} else {
				log.Errorf("ssa type FirstOrCreate err: %v", err)
				return err
			}
		}
		return nil
	})
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

func DeleteType(id int) error {
	if id == -1 {
		return utils.Errorf("delete type from database id is -1")
	}
	db := GetDB()
	return db.Where("id = ?", id).Unscoped().Delete(&IrType{}).Error
}
