package ssadb

import (
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
)

type IrOffset struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`
	// offset
	Offset int64 `json:"offset" gorm:"index"`
	//variable
	VariableName string `json:"variable_name" gorm:"index"`
	IsVariable bool `json:"is_variable" gorm:"index"`
	// value
	ValueID int64 `json:"value_id" gorm:"index"`
}

func CreateOffset() *IrOffset {
	ret := &IrOffset{}
	return ret
}
func SaveIrOffset(idx *IrOffset) {
	start := time.Now()
	defer func() {
		atomic.AddUint64(&_SSAIndexCost, uint64(time.Now().Sub(start).Nanoseconds()))
	}()
	db := GetDB()
	db.Save(idx)
}
