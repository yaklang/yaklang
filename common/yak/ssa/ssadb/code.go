package ssadb

import (
	"context"
	"encoding/json"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/jinzhu/gorm"
)

type IrCode struct {
	gorm.Model

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
	Name             string `json:"name"`
	VerboseName      string `json:"verbose_name"`
	ShortVerboseName string `json:"short_verbose_name"`
	String           string `json:"string" gorm:"type:text"`

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
	Defs  Int64Slice `json:"defs" gorm:"type:text"`
	Users Int64Slice `json:"users" gorm:"type:text"`
	// Phi Chains Relation
	Occulatation Int64Slice `json:"phis" gorm:"type:text"`
	// this is user is call and method is this IR self
	CalledBy Int64Slice `json:"is_called_by" gorm:"type:text"`

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
	TypeID int `json:"type_id"`

	// reference
	Point   int64      `json:"point" gorm:"type:text"`
	Pointer Int64Slice `json:"pointer" gorm:"type:text"`

	// not important information
	ExtraInformation string `json:"extra_information" gorm:"type:text"`
}

func emptyIrCode() *IrCode {
	return &IrCode{}
}

func RequireIrCode(DB *gorm.DB, program string) (uint, *IrCode) {
	db := DB.Model(&IrCode{})
	ircode := emptyIrCode()
	ircode.ProgramName = program
	db.Create(ircode)
	return ircode.ID, ircode
}

func GetIrCodeById(db *gorm.DB, id int64) *IrCode {
	if id == -1 {
		return nil
	}
	return db.Model(&IrCode{}).Where("id = ?", id).First(&IrCode{}).Value.(*IrCode)
}

func GetIrByVariable(db *gorm.DB, program, name string) []*IrCode {
	// var irVariable IrIndex
	ret := make([]*IrCode, 0)
	res := db.Model(&IrIndex{}).Where("(variable_name = ? OR class_name = ?) AND program_name = ?", name, name, program)
	for v := range yieldIrIndex(res, context.Background()) {
		ret = append(ret, v)
	}
	return ret
}

func (r *IrCode) IsEmptySourceCodeHash() bool {
	if r == nil {
		return true
	}
	if r.SourceCodeHash == "" {
		return true
	}
	for _, hash := range []func(...any) string{
		utils.CalcMd5,
		utils.CalcSha1,
		utils.CalcSha256,
	} {
		if r.SourceCodeHash == hash("") {
			return true
		}
	}
	return false
}

func (r *IrCode) GetIdInt64() int64 {
	return int64(r.ID)
}

func (r *IrCode) GetIdInt() int {
	return int(r.ID)
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

func (r *IrCode) GetStartAndEndPositions() (*memedit.MemEditor, memedit.PositionIf, memedit.PositionIf, error) {
	editor, err := GetIrSourceFromHash(r.SourceCodeHash)
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
