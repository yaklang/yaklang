package ssadb

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// irOffsetInsertColumns are the IrOffset application columns written by the
// batched multi-row INSERT below (gorm.Model fields left to SQLite defaults,
// matching the prior per-row db.Save-on-zero-PK = INSERT behavior).
var irOffsetInsertColumns = []string{
	"program_name", "file_hash", "start_offset", "end_offset", "variable_name", "value_id",
}

// irOffsetBatchChunk bounds rows per multi-row INSERT under SQLite's ~999
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

// SaveIrOffsetBatch issues chunked multi-row INSERTs for a batch of offsets.
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
	for start := 0; start < len(clean); start += irOffsetBatchChunk {
		end := start + irOffsetBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		if err := bulkInsertIrOffset(db, clean[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func bulkInsertIrOffset(db *gorm.DB, items []*IrOffset) error {
	if len(items) == 0 {
		return nil
	}
	const cols = 6
	placeholder := "(" + strings.Repeat("?,", cols-1) + "?)"
	values := make([]string, 0, len(items))
	args := make([]interface{}, 0, len(items)*cols)
	for _, it := range items {
		values = append(values, placeholder)
		args = append(args,
			it.ProgramName, it.FileHash, it.StartOffset, it.EndOffset,
			it.VariableName, it.ValueID,
		)
	}
	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		TableIrOffsets,
		strings.Join(irOffsetInsertColumns, ","),
		strings.Join(values, ","),
	)
	return db.Exec(sql, args...).Error
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
