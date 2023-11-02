package ssa

type Opcode string

const (
	OpUnknown Opcode = "unknown"

	OpFunction = "Function"

	OpPhi       = "Phi"
	OpConst     = "Const"
	OpParameter = "Parameter"
	OpUndefined = "Undefined"
	OpCall      = "Call"
	OpUpdate    = "Update"
	OpField     = "Field"
	OpBinOp     = "BinOp"
	OpUnOp      = "UnOp"
	OpReturn    = "Return"
)

func (f *Function) GetOpcode() Opcode { return OpFunction }
func (p *Phi) GetOpcode() Opcode      { return OpPhi }

func (p *Return) GetOpcode() Opcode { return OpReturn }

func (b *BinOp) GetOpcode() Opcode { return OpBinOp }

func (u *UnOp) GetOpcode() Opcode { return OpUnOp }

func (c *Call) GetOpcode() Opcode   { return OpCall }
func (c *Update) GetOpcode() Opcode { return OpUpdate }
func (c *Field) GetOpcode() Opcode  { return OpField }

func (p *ConstInst) GetOpcode() Opcode { return OpConst }
func (p *Parameter) GetOpcode() Opcode { return OpParameter }
func (p *Undefined) GetOpcode() Opcode { return OpUndefined }
