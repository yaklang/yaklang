package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type IrOffset struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`
	// offset
	FileHash    string `json:"file_hash" gorm:"index"`
	StartOffset int64  `json:"start_offset" gorm:"index"`
	EndOffset   int64  `json:"end_offset" gorm:"index"`
	//variable
	VariableID int64 `json:"variable_id"` // this id set when have variable, if not set, this is -1
	// value
	ValueID int64 `json:"value_id"` // this id will set
}

func CreateOffset(rng memedit.RangeIf) *IrOffset {
	ret := &IrOffset{}
	ret.FileHash = rng.GetEditor().GetPureSourceHash()
	ret.StartOffset = int64(rng.GetStartOffset())
	ret.EndOffset = int64(rng.GetEndOffset())
	ret.VariableID = -1
	ret.ValueID = -1
	return ret
}
func SaveIrOffset(idx *IrOffset) {
	db := GetDB()
	db.Save(idx)
}
