package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
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

func GetOffsetByVariable(id int64) []*IrOffset {
	db := GetDB()
	var ir []*IrOffset
	if err := db.Model(&IrOffset{}).Where("variable_id = ?", id).Find(&ir).Error; err != nil {
		return nil
	}
	return ir
}

func (r *IrOffset) GetStartAndEndPositions() (*memedit.MemEditor, memedit.PositionIf, memedit.PositionIf, error) {
	editor, err := GetIrSourceFromHash(r.FileHash)
	if err != nil {
		return nil, nil, nil, utils.Errorf("GetStartAndEndPositions failed: %v", err)
	}
	start, end := editor.GetPositionByOffset(int(r.StartOffset)), editor.GetPositionByOffset(int(r.EndOffset))
	return editor, start, end, nil
}
