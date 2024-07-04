package ssadb

import (
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type IrType struct {
	gorm.Model
	Kind             int    `json:"kind"`
	String           string `json:"string"`
	ExtraInformation string `json:"extra_information"`
	Hash             string `json:"hash" gorm:"unique_index"`
}

func (t *IrType) CalcHash() string {
	return utils.CalcSha1(t.Kind, t.String, t.ExtraInformation)
}

func (t *IrType) BeforeUpdate() error {
	t.Hash = t.CalcHash()
	return nil
}

func (t *IrType) BeforeSave() error {
	t.Hash = t.CalcHash()
	return nil
}

func (t *IrType) BeforeCreate() error {
	t.Hash = t.CalcHash()
	return nil
}

func SaveType(kind int, str string, extra string) int {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSASaveTypeCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()

	db := GetDB()
	irType := &IrType{
		Kind:             kind,
		String:           str,
		ExtraInformation: extra,
	}
	if db := db.Where(irType).FirstOrCreate(irType); db.Error != nil {
		log.Errorf("SaveType error: %v", db.Error)
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
