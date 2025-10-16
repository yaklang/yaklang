package ssadb

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
)

type IrOffset struct {
	gorm.Model

	ProgramName string `json:"program_name" gorm:"index"`
	// offset
	FileHash    string `json:"file_hash" gorm:"index"`
	StartOffset int64  `json:"start_offset" gorm:"index"`
	EndOffset   int64  `json:"end_offset" gorm:"index"`
	//variable
	VariableName string `json:"variable_name" gorm:"index"` // this id set when have variable
	// value
	ValueID int64 `json:"value_id" gorm:"index"` // this id will set
}

func CreateOffset(rng *memedit.Range, projectName string) *IrOffset {
	ret := &IrOffset{}
	ret.FileHash = rng.GetEditor().GetIrSourceHash()
	ret.ProgramName = projectName
	ret.StartOffset = int64(rng.GetStartOffset())
	ret.EndOffset = int64(rng.GetEndOffset())
	ret.VariableName = ""
	ret.ValueID = -1
	return ret
}
func SaveIrOffset(db *gorm.DB, idx *IrOffset) {
	// db.Save(idx)
	var err error
	ssaprofile.ProfileAdd(true, "Database.IrOffset", func() {
		err = db.Save(idx).Error
	})
	_ = err
}

func GetOffsetByVariable(name string, valueID int64) []*IrOffset {
	db := GetDB()
	var ir []*IrOffset
	if err := db.Model(&IrOffset{}).Where("variable_name = ? and value_id = ?", name, valueID).Find(&ir).Error; err != nil {
		return nil
	}
	return ir
}

func GetValueBeforeEndOffset(DB *gorm.DB, rng *memedit.Range) (int64, error) {
	// get the last ir code before the end offset, and the source code hash must be the same
	hash := rng.GetEditor().GetIrSourceHash()
	db := DB.Model(&IrOffset{})
	db = db.Where("end_offset <= ? and  file_hash = ?", rng.GetEndOffset(), hash)
	var ir IrOffset
	if err := db.Order("end_offset desc").First(&ir).Error; err != nil {
		return -1, err
	}
	return int64(ir.ValueID), nil
}

func (r *IrOffset) GetStartAndEndPositions() (*memedit.MemEditor, *memedit.Position, *memedit.Position, error) {
	editor, err := GetEditorByHash(r.FileHash)
	if err != nil {
		return nil, nil, nil, utils.Errorf("GetStartAndEndPositions failed: %v", err)
	}
	start, end := editor.GetPositionByOffset(int(r.StartOffset)), editor.GetPositionByOffset(int(r.EndOffset))
	return editor, start, end, nil
}
