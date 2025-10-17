package ssadb

import (
	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
)

type IrType struct {
	gorm.Model
	TypeId           uint64 `json:"type_id" gorm:"index"`
	Kind             int    `json:"kind"`
	ProgramName      string `json:"program_name"`
	String           string `json:"string" gorm:"type:text"`
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
	// Hash             string `json:"hash" gorm:"unique_index"`
}

func (t *IrType) GetIdInt64() int64 {
	return int64(t.TypeId)
}

func (t *IrType) CalcHash(ex ...string) string {
	return utils.CalcSha1(t.ProgramName, t.Kind, t.String, t.ExtraInformation, ex)
}

func (ir *IrType) Save(db *gorm.DB) error {
	var err error
	ssaprofile.ProfileAdd(true, "Database.SaveIrType", func() {
		err = db.Save(ir).Error
	})
	return err
}

func EmptyIrType(progName string, id uint64) *IrType {
	return &IrType{
		ProgramName: progName,
		TypeId:      id,
	}
}

func GetIrTypeItemById(db *gorm.DB, progName string, id int64) *IrType {
	if id == -1 {
		return nil
	}
	// check cache
	ir := &IrType{}
	// db = db.Debug()
	if db := db.Model(&IrType{}).
		Where("type_id = ?", id).
		Where("program_name = ?", progName).
		First(ir); db.Error != nil {
		return nil
	}
	return ir
}

// var saveTypeMutex sync.Mutex

// func SaveType(kind int, str, extra, progName string) int {
// 	// return -1
// 	start := time.Now()
// 	defer func() {
// 		atomic.AddUint64(&_SSASaveTypeCost, uint64(time.Since(start).Nanoseconds()))
// 	}()

// 	irType := IrType{
// 		Kind:             kind,
// 		ProgramName:      progName,
// 		String:           str,
// 		ExtraInformation: extra,
// 	}
// 	irType.Hash = irType.CalcHash()
// 	if ret, ok := TypeMap.Get(irType.Hash); ok {
// 		return ret
// 	} else {
// 		saveType(&irType)
// 		TypeMap.Set(irType.Hash, int(irType.ID))
// 	}
// 	return int(irType.ID)
// }

// var TypeMap *utils.SafeMap[int] = utils.NewSafeMap[int]()

// func saveType(irType *IrType) error {
// 	saveTypeMutex.Lock()
// 	defer saveTypeMutex.Unlock()

// 	db := GetDB()
// 	err := db.Where("hash = ?", irType.Hash).FirstOrCreate(&irType).Error
// 	if err != nil {
// 		log.Errorf("ssa type FirstOrCreate err: %v", err)
// 		return err
// 	}
// 	return nil
// }

// func GetType(id int) (int, string, string, error) {
// 	if id == -1 {
// 		return 0, "", "", utils.Errorf("get type from database id is -1")
// 	}
// 	db := GetDB()
// 	irType := &IrType{}
// 	if db := db.First(irType, id); db.Error != nil {
// 		return 0, "", "", db.Error
// 	}
// 	return irType.Kind, irType.String, irType.ExtraInformation, nil
// }

func DeleteIrType(db *gorm.DB, id []int64) error {
	// log.Errorf("DeleteIrType: %d", len(id))
	if len(id) == 0 {
		return utils.Errorf("delete type from database id is empty")
	}
	return utils.GormTransaction(db, func(tx *gorm.DB) error {
		// split each 999
		for i := 0; i < len(id); i += 999 {
			end := i + 999
			if end > len(id) {
				end = len(id)
			}
			tx.Where("id IN (?)", id[i:end]).Unscoped().Delete(&IrType{})
		}
		return nil
	})
}
