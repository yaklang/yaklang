package ssadb

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
	IsFunction       bool     `json:"is_function"`
	FormalArgs       []uint64 `json:"formal_args"`
	IsVariadic       bool     `json:"is_variadic"`
	ReturnCodes      []uint64 `json:"return_codes"`
	SideEffects      []uint64 `json:"side_effects"`
	IsExternal       bool     `json:"is_external"`
	IsCalledBy       []uint64 `json:"is_called_by"`
	CodeBlocks       []uint64 `json:"code_blocks"`
	EnterBlock       []uint64 `json:"enter_block"`
	ExitBlock        []uint64 `json:"exit_block"`
	DeferBlock       []uint64 `json:"defer_block"`
	ChildrenFunction []uint64 `json:"children_function"`

	// block
	IsBlock   bool     `json:"is_block"`
	PredBlock []uint64 `json:"pred_block"`
	SuccBlock []uint64 `json:"succ_block"`
	Phis      []uint64 `json:"phis_in_block"`

	// Use-Def Chains Relation
	Defs  []uint64 `json:"defs"`
	Users []uint64 `json:"users"`

	// OOP Supporting
	ObjectMembers []uint64 `json:"object_members"`
	ObjectParent  uint64   `json:"object_parent"`

	// Maskable
	MaskedCodes []uint64 `json:"masked_codes"`
	IsMasked    bool     `json:"is_masked"`
	IsMaskedBy  uint64   `json:"is_masked_by"`

	// Called
	IsCalled   bool     `json:"is_called"`
	ActualArgs []uint64 `json:"actual_args"`
}
