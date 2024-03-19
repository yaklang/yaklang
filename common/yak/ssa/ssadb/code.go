package ssadb

import (
	"github.com/jinzhu/gorm"
	"sync"
)

type IrCode struct {
	ID uint64 `json:"id" gorm:"primary_key,auto_increment"`

	ProgramName string `json:"program_name" gorm:"index"`
	PackageName string `json:"package_name" gorm:"index"`

	// source code
	SourceCodeFile      string `json:"source_code_file"`
	SourceCodeStartLine int    `json:"source_code_start_line"`
	SourceCodeEndLine   int    `json:"source_code_end_line"`
	SourceCodeStartCol  int    `json:"source_code_start_col"`
	SourceCodeEndCol    int    `json:"source_code_end_col"`
	SourceCodeHash      string `json:"source_code_hash"`

	Opcode     int64  `json:"opcode"`
	OpcodeName string `json:"opcode_name"`

	// basic info
	Name             string `json:"name"`
	VerboseName      string `json:"verbose_name"`
	ShortVerboseName string `json:"short_verbose_name"`

	// any IrCode in one function
	ParentFunction uint64 `json:"parent_function"`
	CurrentBlock   uint64 `json:"current_block"`

	// FunctionDefs
	IsFunction       bool        `json:"is_function"`
	FormalArgs       Uint64Slice `json:"formal_args" gorm:"type:text"`
	IsVariadic       bool        `json:"is_variadic"`
	ReturnCodes      Uint64Slice `json:"return_codes" gorm:"type:text"`
	SideEffects      Uint64Slice `json:"side_effects" gorm:"type:text"`
	IsExternal       bool        `json:"is_external"`
	IsCalledBy       Uint64Slice `json:"is_called_by" gorm:"type:text"`
	CodeBlocks       Uint64Slice `json:"code_blocks" gorm:"type:text"`
	EnterBlock       uint64      `json:"enter_block"`
	ExitBlock        uint64      `json:"exit_block"`
	DeferBlock       uint64      `json:"defer_block"`
	ChildrenFunction Uint64Slice `json:"children_function" gorm:"type:text"`

	// block
	IsBlock   bool        `json:"is_block"`
	PredBlock Uint64Slice `json:"pred_block" gorm:"type:text"`
	SuccBlock Uint64Slice `json:"succ_block" gorm:"type:text"`
	Phis      Uint64Slice `json:"phis_in_block" gorm:"type:text"`

	// Use-Def Chains Relation
	Defs  Uint64Slice `json:"defs" gorm:"type:text"`
	Users Uint64Slice `json:"users" gorm:"type:text"`

	// OOP Supporting
	ObjectMembers Uint64Slice `json:"object_members" gorm:"type:text"`
	ObjectParent  uint64      `json:"object_parent"`

	// Maskable
	MaskedCodes Uint64Slice `json:"masked_codes" gorm:"type:text"`
	IsMasked    bool        `json:"is_masked"`
	IsMaskedBy  uint64      `json:"is_masked_by"`

	// Called
	IsCalled   bool        `json:"is_called"`
	ActualArgs Uint64Slice `json:"actual_args" gorm:"type:text"`
}

func emptyIrCode() *IrCode {
	return &IrCode{}
}

var verifyExisted = new(sync.Once)

func RequireIrCode(db *gorm.DB) (uint64, *IrCode) {
	verifyExisted.Do(func() {
		if !db.HasTable(&IrCode{}) {
			db.AutoMigrate(&IrCode{})
		}
	})
	db = db.Model(&IrCode{})
	// save new ircode
	ircode := emptyIrCode()
	db.Create(ircode)
	return ircode.ID, ircode
}
