package ssadb

import (
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// irOffsetBatchChunk bounds rows per CreateInBatches call under SQLite's ~999
// host-parameter limit: 150 rows * 6 cols = 900.
const irOffsetBatchChunk = 150

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

func (*IrOffset) TableName() string {
	return TableIrOffsets
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
	_ = db.Save(idx).Error
}

// SaveIrOffsetBatch issues chunked batched INSERTs for a batch of offsets.
// ir_offsets has no UNIQUE constraint and recompile deletes the program's rows
// first (ssadb.DeleteProgramIrCode), so this is a pure INSERT — not an upsert —
// matching the prior per-row SaveIrOffset path, but in one statement per chunk
// instead of N round-trips.
func SaveIrOffsetBatch(db *gorm.DB, items []*IrOffset) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	clean := make([]*IrOffset, 0, len(items))
	for _, it := range items {
		if it != nil {
			clean = append(clean, it)
		}
	}
	if r := db.CreateInBatches(clean, irOffsetBatchChunk); r.Error != nil {
		return r.Error
	}
	return nil
}

func GetOffsetByVariable(name string, valueID int64, programName string) []*IrOffset {
	db := GetDB()
	var ir []*IrOffset
	query := db.Model(&IrOffset{}).Where("variable_name = ? and value_id = ?", name, valueID)
	if p := strings.TrimSpace(programName); p != "" {
		query = query.Where("program_name = ?", p)
	}
	if err := query.Find(&ir).Error; err != nil {
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
	if r == nil {
		return nil, nil, nil, nil
	}
	if strings.TrimSpace(r.FileHash) == "" {
		// Some synthetic variables (for example dependency/config placeholders)
		// intentionally have no backing source file.
		return nil, nil, nil, nil
	}

	editor, err := GetEditorByHash(r.FileHash)
	if err != nil {
		return nil, nil, nil, utils.Errorf("GetStartAndEndPositions failed: %v", err)
	}
	start, end := editor.GetPositionByOffset(int(r.StartOffset)), editor.GetPositionByOffset(int(r.EndOffset))
	return editor, start, end, nil
}
