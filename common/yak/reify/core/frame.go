package core

// FrameModel describes how one architecture lays out a call frame. A parser
// fills it from the facts its architecture defines, so a pass can name the stack
// and frame registers without matching architecture-specific spellings.
//
// The register names are the ones the parser writes into LowIR operands, not the
// names the architecture's assembly uses: s390x writes "r15" for its stack
// pointer, loong64 writes "r3", mips writes "sp".
//
// An empty field means the architecture does not define the role. A pass that
// needs a missing role leaves the function unchanged instead of guessing.
type FrameModel struct {
	// StackPointer names the register a call adjusts to allocate a frame.
	StackPointer string `json:"stack_pointer,omitempty"`
	// FramePointer names the register that holds the frame base. It is empty
	// when the platform does not use one.
	FramePointer string `json:"frame_pointer,omitempty"`
	// ReturnAddress names the register that holds the caller's address on a
	// link-register architecture. It is empty when the call instruction stores
	// the address on the stack.
	ReturnAddress string `json:"return_address,omitempty"`
	// Flags names the pseudo register that carries the condition flags.
	Flags string `json:"flags,omitempty"`
	// CalleeSaved lists the registers a callee must preserve, including the
	// frame pointer when one is used.
	CalleeSaved []string `json:"callee_saved,omitempty"`
	// StackGrowth is -1 for the usual downward-growing stack and +1 for an
	// upward-growing one. Zero means unknown.
	StackGrowth int8 `json:"stack_growth,omitempty"`
}

// CanaryIntrinsic names the neutral intrinsic a lifter emits when it recognizes
// that one value is the stack-protector guard. The read itself is
// architecture-specific: x86 reads fs:0x28, arm64 reads tpidr_el0+0x28, and the
// RISC-like targets load __stack_chk_guard through the GOT. Each lifter
// recognizes its own form and emits this one marker, so the canary pass stays
// architecture-neutral.
const CanaryIntrinsic = "canary"

// TLSBaseIntrinsic marks the value a lifter recognizes as the thread pointer the
// stack guard is stored behind. x86 reaches it through the fs or gs segment base;
// arm64 reads tpidr_el0. The pass adds the offset from the canary model, so the
// read shape stays neutral.
const TLSBaseIntrinsic = "tls.base"

// FlagsIntrinsicPrefix names the namespace a lifter uses when one instruction
// updates the condition flags: flags.cmp, flags.add, flags.ccmp. The update's
// operands are the compared values, so a consumer can rebuild the comparison
// without knowing which architecture produced it.
const FlagsIntrinsicPrefix = "flags."

// ConditionIntrinsicPrefix names the namespace a lifter uses when one
// instruction reads a condition code: cond.e, cond.eq, cond.ne. The operand is
// the flag value the instruction reads.
const ConditionIntrinsicPrefix = "cond."

// Register naming modes. A converter needs to know how one machine register name
// maps to one SSA variable, and the rule is architecture-specific: x86 splits a
// register into 8, 16 and 32-bit views of one 64-bit variable, AArch64 shares one
// variable between Wn and Xn, and every other architecture treats a name as one
// register.
const (
	RegisterNamingX86     = "x86"
	RegisterNamingAArch64 = "aarch64"
	RegisterNamingPlain   = "plain"
)

// RegisterModel describes how the target names its registers and passes
// arguments. A converter reads it instead of switching on the architecture.
type RegisterModel struct {
	// Naming selects the alias rule: RegisterNamingX86, RegisterNamingAArch64
	// or RegisterNamingPlain.
	Naming string `json:"naming,omitempty"`
	// Width is the default integer register width.
	Width uint16 `json:"width,omitempty"`
	// Flags names the condition flag register, empty when the target has none.
	Flags string `json:"flags,omitempty"`
	// FlagsWidth is the width of the flag variable. It is separate from Width
	// because a flag word is not an integer register.
	FlagsWidth uint16 `json:"flags_width,omitempty"`
	// Arguments lists the integer argument registers in order. An empty list
	// means the target passes every argument on the stack.
	Arguments []string `json:"arguments,omitempty"`
	// Return names the register that carries a scalar result.
	Return string `json:"return,omitempty"`
}

// ArchFacts carries the architecture facts a LowIR pass reads. A parser fills
// them once per LowIR, so a pass never inspects an architecture name to decide
// which registers or intrinsics to look for.
type ArchFacts struct {
	Frame     FrameModel    `json:"frame,omitempty"`
	Registers RegisterModel `json:"registers,omitempty"`
	Canary    CanaryModel   `json:"canary,omitempty"`
}

// Facts returns the architecture facts of this program.
func (ir *LowIR) Facts() ArchFacts {
	if ir == nil {
		return ArchFacts{}
	}
	return ArchFacts{Frame: ir.Frame, Registers: ir.Registers, Canary: ir.Canary}
}

// EntryRegisters lists the registers that hold an ABI-defined value when a
// function starts: the stack pointer, the frame pointer and the return address.
// A converter binds them before it converts the body, so a prologue that saves or
// moves them sees a variable rather than an unknown register.
func (f FrameModel) EntryRegisters() []string {
	result := make([]string, 0, 3)
	for _, name := range []string{f.StackPointer, f.FramePointer, f.ReturnAddress} {
		if name == "" {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing == name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, name)
		}
	}
	return result
}

// SavedRegisters lists the registers a prologue preserves: the callee-saved set
// plus the return address register, which a link-register target has to spill
// before it makes another call.
func (f FrameModel) SavedRegisters() []string {
	if f.ReturnAddress == "" {
		return f.CalleeSaved
	}
	for _, name := range f.CalleeSaved {
		if name == f.ReturnAddress {
			return f.CalleeSaved
		}
	}
	result := make([]string, 0, len(f.CalleeSaved)+1)
	result = append(result, f.CalleeSaved...)
	return append(result, f.ReturnAddress)
}

// CanaryModel describes where one architecture reads the stack-protector guard.
// A lifter recognizes the read and emits the neutral CanaryIntrinsic, so the
// canary pass never has to know a segment or thread-pointer spelling.
type CanaryModel struct {
	// Kind selects how the guard is reached: "segment" reads a segment base at a
	// fixed offset, "tls" reads a thread pointer register at a fixed offset, and
	// "symbol" loads a named global through the GOT.
	Kind string `json:"kind,omitempty"`
	// Register names the segment register or the thread pointer register.
	Register string `json:"register,omitempty"`
	// Offset is the byte offset from the base the guard is stored at.
	Offset uint64 `json:"offset,omitempty"`
	// Symbol is the global that holds the guard for the symbol kind.
	Symbol string `json:"symbol,omitempty"`
}

// Canary read kinds.
const (
	CanarySegment = "segment"
	CanaryTLS     = "tls"
	CanarySymbol  = "symbol"
)

// IsLandingPadIntrinsic reports whether one intrinsic is a control-flow
// enforcement landing pad (x86 endbr32/endbr64 and their ARM equivalents). A
// landing pad only marks a valid indirect-call target and has no data flow, so a
// frame pass treats it as noise.
func IsLandingPadIntrinsic(name string) bool {
	for _, suffix := range []string{".endbr32", ".endbr64", ".bti", ".paciasp", ".pacibsp"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
