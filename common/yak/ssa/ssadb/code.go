package ssadb

import (
	"encoding/json"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/jinzhu/gorm"
)

type IrCode struct {
	gorm.Model
	CodeID int64 `json:"code_id" gorm:"index"`

	ProgramName string `json:"program_name" gorm:"index"`
	Version     string `json:"package_version" gorm:"index"`

	// source code
	SourceCodeStartOffset int64  `json:"source_code_start_offset"`
	SourceCodeEndOffset   int64  `json:"source_code_end_offset"`
	SourceCodeHash        string `json:"source_code_hash"` // default md5

	// opcode
	Opcode     int64  `json:"opcode"`
	OpcodeName string `json:"opcode_name"`

	// just for binary and unary operator
	OpcodeOperator string `json:"opcode_operator"`

	// basic info
	Name              string `json:"name"`
	VerboseName       string `json:"verbose_name"`
	ShortVerboseName  string `json:"short_verbose_name"`
	String            string `json:"string" gorm:"type:text"`
	ReadableName      string `json:"readable_name" gorm:"type:text"`
	ReadableNameShort string `json:"readable_name_short"`
	// any IrCode in one block inner one  function
	CurrentBlock    int64 `json:"current_block"`
	CurrentFunction int64 `json:"current_function"`

	// FunctionDefs
	IsFunction     bool       `json:"is_function"`
	FormalArgs     Int64Slice `json:"formal_args" gorm:"type:text"`
	FreeValues     Int64Slice `json:"free_values" gorm:"type:text"`
	MemberCallArgs Int64Slice `json:"formal_member_call_args" gorm:"type:text"`

	SideEffects      Int64Slice `json:"side_effects" gorm:"type:text"`
	IsVariadic       bool       `json:"is_variadic"`
	ReturnCodes      Int64Slice `json:"return_codes" gorm:"type:text"`
	IsExternal       bool       `json:"is_external"`
	CodeBlocks       Int64Slice `json:"code_blocks" gorm:"type:text"`
	EnterBlock       int64      `json:"enter_block"`
	ExitBlock        int64      `json:"exit_block"`
	DeferBlock       int64      `json:"defer_block"`
	ChildrenFunction Int64Slice `json:"children_function" gorm:"type:text"`
	ParentFunction   int64      `json:"parent_function"`

	// block
	IsBlock   bool       `json:"is_block"`
	PredBlock Int64Slice `json:"pred_block" gorm:"type:text"`
	SuccBlock Int64Slice `json:"succ_block" gorm:"type:text"`
	Phis      Int64Slice `json:"phis_in_block" gorm:"type:text"`

	// Use-Def Chains Relation
	HasDefs bool       `json:"has_defs"`
	Users   Int64Slice `json:"users" gorm:"type:text"`
	// Phi Chains Relation
	Occulatation Int64Slice `json:"phis" gorm:"type:text"`

	// OOP Supporting
	IsObject       bool
	ObjectMembers  Int64Map `json:"object_members" gorm:"type:text"`
	IsObjectMember bool
	ObjectParent   int64 `json:"object_parent"`
	ObjectKey      int64 `json:"object_key"`

	// Maskable
	MaskedCodes Int64Slice `json:"masked_codes" gorm:"type:text"`
	IsMasked    bool       `json:"is_masked"`

	// Variable
	Variable StringSlice `json:"variable" gorm:"type:text"`

	// compile hash means: hash[ (file-content)+(program-name)+(package-name)+(program-index) ]
	ProgramCompileHash string `json:"program_compile_hash" gorm:"index"`

	// type
	TypeID int64 `json:"type_id"`

	// reference
	Point   int64      `json:"point" gorm:"type:text"`
	Pointer Int64Slice `json:"pointer" gorm:"type:text"`

	// not important information
	ExtraInformation string `json:"extra_information" gorm:"type:text"`

	ConstType string `json:"const_type" gorm:"index"`
}

func EmptyIrCode(progName string, id int64) *IrCode {
	return &IrCode{
		ProgramName: progName,
		CodeID:      id,
	}
}

func GetIrCodeItemById(db *gorm.DB, progName string, id int64) *IrCode {
	if id == -1 {
		return nil
	}
	// check cache
	ir := &IrCode{}
	if db := db.Model(&IrCode{}).
		Where("code_id = ?", id).
		Where("program_name = ?", progName).
		First(ir); db.Error != nil {
		return nil
	}
	return ir
}

func (ir *IrCode) Save(db *gorm.DB) error {
	var err error
	ssaprofile.ProfileAdd(true, "Database.SaveIrCode", func() {
		err = db.Save(ir).Error
	})
	return err
}

func (r *IrCode) IsEmptySourceCodeHash() bool {
	if r == nil {
		return true
	}
	if r.SourceCodeHash == "" {
		return true
	}
	for _, hash := range []func(any) string{
		codec.Md5,
		codec.Sha1,
		codec.Sha256,
	} {
		hashx := hash("")
		if r.SourceCodeHash == hashx {
			return true
		}
	}
	return false
}

func (r *IrCode) GetIdInt64() int64 {
	return int64(r.CodeID)
}

func (r *IrCode) GetIdInt() int {
	return int(r.CodeID)
}

func (r *IrCode) SetExtraInfo(params map[string]any) {
	results, err := json.Marshal(params)
	if err != nil {
		log.Warnf("BUG: cannot fetch instruction's extra info: %v, origin: %s", err, spew.Sdump(params))
	}
	r.ExtraInformation = string(results)
}

func (r *IrCode) GetExtraInfo() map[string]any {
	results := make(map[string]any)
	if r.ExtraInformation == "" {
		return results
	}
	if err := json.Unmarshal([]byte(r.ExtraInformation), &results); err != nil {
		log.Warnf("BUG: cannot fetch instruction's extra info: %v, origin: %v", err, r.ExtraInformation)
	}
	return results
}

func (r *IrCode) GetStartAndEndPositions() (*memedit.MemEditor, *memedit.Position, *memedit.Position, error) {
	editor, err := GetEditorByHash(r.SourceCodeHash)
	if err != nil {
		return nil, nil, nil, utils.Errorf("GetStartAndEndPositions failed: %v", err)
	}
	start, end := editor.GetPositionByOffset(int(r.SourceCodeStartOffset)), editor.GetPositionByOffset(int(r.SourceCodeEndOffset))
	return editor, start, end, nil
}

func (r *IrCode) GetSourceCode() string {
	editor, start, end, err := r.GetStartAndEndPositions()
	if err != nil {
		log.Warnf("GetSourceCode failed: %v", err)
		return ""
	}
	return editor.GetWordTextFromRange(editor.GetRangeByPosition(start, end))
}

func (r *IrCode) GetSourceCodeContext(n int) string {
	editor, start, end, err := r.GetStartAndEndPositions()
	if err != nil {
		log.Warnf("GetSourceCodeContext failed: %v", err)
		return ""
	}
	result, err := editor.GetContextAroundRange(start, end, n)
	if err != nil {
		log.Warnf("editor.GetContextAroundRange(start, end, %v) failed: %v", n, err)
		return ""
	}
	return result
}

func DeleteIrCode(DB *gorm.DB, id ...int64) error {
	// log.Errorf("DeleteIrType: %d", len(id))
	if len(id) == 0 {
		return utils.Errorf("delete type from database id is empty")
	}
	return utils.GormTransaction(DB, func(tx *gorm.DB) error {
		// split each 999
		for i := 0; i < len(id); i += 999 {
			end := i + 999
			if end > len(id) {
				end = len(id)
			}
			tx.Where("id IN (?)", id[i:end]).Unscoped().Delete(&IrCode{})
		}
		return nil
	})
}
