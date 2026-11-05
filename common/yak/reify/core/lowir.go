package core

// LowIR is the machine-level program representation. It is a concrete value,
// not an interface: every parser produces the same Function, Block and
// Instruction shapes. Function IDs are local to one LowIR; block and value IDs
// are local to one Function. Zero denotes an absent ID. Values have single
// definitions. Registers and memory remain explicit effects rather than
// implicit mutable SSA values.
type LowIR struct {
	Target    Target     `json:"target"`
	Functions []Function `json:"functions"`
}

// Function is one decoded unit. Address is its entry address. The current
// parsers create one function for each ParseRequest region.
type Function struct {
	ID      FunctionID `json:"id"`
	Address uint64     `json:"address"`
	Blocks  []Block    `json:"blocks"`
}

// Block contains ordered operations and ends in a control-transfer operation.
// Targets on that operation identify known successors within the same function.
type Block struct {
	ID           BlockID       `json:"id"`
	Address      uint64        `json:"address"`
	Instructions []Instruction `json:"instructions"`
}

type FunctionID uint64
type BlockID uint64
type ValueID uint64

// Instruction is a normalized operation. Targets describes control targets, or
// Phi predecessors in operand order. Effects and instruction order must be
// preserved unless a transformation proves a change valid.
type Instruction struct {
	Kind     Opcode    `json:"kind"`
	Operands []Operand `json:"operands,omitempty"`
	Results  []Value   `json:"results,omitempty"`
	Targets  []BlockID `json:"targets,omitempty"`
	Effects  Effect    `json:"effects,omitempty"`
	Address  uint64    `json:"address"`
}

type Opcode string

const (
	OpConstant      Opcode = "constant"
	OpCopy          Opcode = "copy"
	OpPhi           Opcode = "phi"
	OpSelect        Opcode = "select"
	OpNop           Opcode = "nop"
	OpReadRegister  Opcode = "read-register"
	OpWriteRegister Opcode = "write-register"
	OpLoad          Opcode = "load"
	OpStore         Opcode = "store"
	OpAdd           Opcode = "add"
	OpSub           Opcode = "sub"
	OpMul           Opcode = "mul"
	OpUDiv          Opcode = "udiv"
	OpSDiv          Opcode = "sdiv"
	OpNeg           Opcode = "neg"
	OpNot           Opcode = "not"
	OpAnd           Opcode = "and"
	OpOr            Opcode = "or"
	OpXor           Opcode = "xor"
	OpShl           Opcode = "shl"
	OpLShr          Opcode = "lshr"
	OpAShr          Opcode = "ashr"
	OpEqual         Opcode = "eq"
	OpULess         Opcode = "ult"
	OpSLess         Opcode = "slt"
	OpZeroExtend    Opcode = "zext"
	OpSignExtend    Opcode = "sext"
	OpTruncate      Opcode = "trunc"
	OpCall          Opcode = "call"
	OpIntrinsic     Opcode = "intrinsic"
	OpJump          Opcode = "jump"
	OpBranch        Opcode = "branch"
	OpReturn        Opcode = "return"
	OpTrap          Opcode = "trap"
)

type TypeKind string

const (
	TypeInteger TypeKind = "integer"
	TypeFloat   TypeKind = "float"
	TypePointer TypeKind = "pointer"
	TypeOpaque  TypeKind = "opaque"
)

// ValueType retains machine precision. Integer signedness belongs to operations
// (for example SDiv/UDiv), not to every bit pattern.
type ValueType struct {
	Kind TypeKind `json:"kind"`
	Bits uint16   `json:"bits"`
}

type Value struct {
	ID   ValueID   `json:"id"`
	Type ValueType `json:"type"`
}

type OperandKind string

const (
	OperandValue    OperandKind = "value"
	OperandConstant OperandKind = "constant"
	OperandRegister OperandKind = "register"
	OperandSymbol   OperandKind = "symbol"
)

// Operand is a tagged payload. Value is used by OperandValue; Literal contains
// little-endian bits for OperandConstant; Name identifies a register or symbol.
// Literal encoding is independent of the target's memory byte order.
type Operand struct {
	Kind    OperandKind `json:"kind"`
	Type    ValueType   `json:"type"`
	Value   ValueID     `json:"value,omitempty"`
	Literal []byte      `json:"literal,omitempty"`
	Name    string      `json:"name,omitempty"`
}

type Effect uint32

const (
	EffectNone          Effect = 0
	EffectReadMemory    Effect = 1 << 0
	EffectWriteMemory   Effect = 1 << 1
	EffectReadRegister  Effect = 1 << 2
	EffectWriteRegister Effect = 1 << 3
	EffectCall          Effect = 1 << 4
	EffectMayTrap       Effect = 1 << 5
	EffectVolatile      Effect = 1 << 6
	EffectAtomic        Effect = 1 << 7
	EffectUnknown       Effect = 1 << 8
)
