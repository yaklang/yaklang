package ssadb

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/utils"
)

type IrType struct {
	gorm.Model
	Kind             int    `json:"kind"`
	ProgramName      string `json:"program_name"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
	Hash             string `json:"hash" gorm:"unique_index"`
}

func (t *IrType) CalcHash() string {
	return utils.CalcSha1(t.ProgramName, t.Kind, t.String, t.ExtraInformation)
}

var saveTypeMutex sync.Mutex

func SaveType(kind int, str, extra, progName string) int {
	// return -1
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSASaveTypeCost, uint64(time.Since(start).Nanoseconds()))
	}()

	irType := IrType{
		Kind:             kind,
		ProgramName:      progName,
		String:           str,
		ExtraInformation: extra,
	}
	irType.Hash = irType.CalcHash()
	if ret, ok := TypeMap.Get(irType.Hash); ok {
		return ret
	} else {
		saveType(&irType)
		TypeMap.Set(irType.Hash, int(irType.ID))
	}
	return int(irType.ID)
}

var TypeMap *utils.SafeMap[int] = utils.NewSafeMap[int]()

func saveType(irType *IrType) error {
	saveTypeMutex.Lock()
	defer saveTypeMutex.Unlock()

	db := GetDB()
	err := db.Where("hash = ?", irType.Hash).FirstOrCreate(&irType).Error
	if err != nil {
		log.Errorf("ssa type FirstOrCreate err: %v", err)
		return err
	}
	return nil
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
