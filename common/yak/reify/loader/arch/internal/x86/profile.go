package x86

import (
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Profile is the per-mode difference between the 32-bit and the 64-bit x86
// parsers. Everything else — decoding, block construction, lifting — is shared,
// because the instruction encoding and the LowIR shape are the same; only the
// default operand width, register names and the addressing base differ.
type Profile struct {
	// Kind is the architecture this profile decodes.
	Kind core.Architecture
	// Name is the short architecture name used in diagnostics.
	Name string
	// Mode is the x86asm processor mode: 32 or 64.
	Mode int
	// Bits is the default operand and address width.
	Bits uint16
	// StackPointer, StackFrame and InstructionPointer name the stack and frame
	// registers. InstructionPointer is empty for 32-bit code, which has no
	// program-counter-relative addressing.
	StackPointer, StackFrame, InstructionPointer string
	// Accumulator and AccumulatorHigh name the multiply and divide registers.
	Accumulator, AccumulatorHigh string
	// Accumulator8 and Accumulator8High are the 8-bit views of the accumulator.
	Accumulator8, Accumulator8High string
	// CountRegister names the register used by LOOP, JCXZ and their aliases.
	CountRegister string
	// Flags is the pseudo register that carries the arithmetic flags.
	Flags string
	// Endbr marks the CET landing-pad mnemonics this profile accepts.
	Endbr string
	// CalleeSaved lists the registers a callee must preserve, including the
	// frame pointer.
	CalleeSaved []string
	// Canary describes where the stack protector reads its guard.
	Canary core.CanaryModel
}

// Registers describes register naming and the integer argument registers of this
// profile. The 64-bit System V ABI passes six arguments in registers; the 32-bit
// cdecl ABI passes every argument on the stack.
func (p Profile) Registers() core.RegisterModel {
	return p.RegistersFor(core.Target{})
}

// RegistersFor selects the argument registers of the target system. Windows passes
// its first four integer arguments in rcx, rdx, r8 and r9, while the System V
// targets use rdi, rsi, rdx and rcx.
func (p Profile) RegistersFor(target core.Target) core.RegisterModel {
	if p.Mode == 64 && target.OS == core.OSWindows {
		return core.RegisterModel{
			Naming:     core.RegisterNamingX86,
			Width:      64,
			Flags:      "rflags",
			FlagsWidth: 64,
			Arguments:  []string{"rcx", "rdx", "r8", "r9"},
			Return:     "rax",
		}
	}
	return p.registers()
}

func (p Profile) registers() core.RegisterModel {
	model := core.RegisterModel{
		Naming:     core.RegisterNamingX86,
		Width:      p.Bits,
		Flags:      p.Flags,
		FlagsWidth: 64,
	}
	if p.Mode == 64 {
		model.Arguments = []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
		model.Return = "rax"
		return model
	}
	// A 32-bit result is the low half of the same variable. The cdecl
	// convention passes every argument on the stack.
	model.Return = "eax"
	model.StackArguments = true
	if p.Mode == 16 {
		model.Return = "ax"
	}
	return model
}

// Frame describes the call frame for this profile. The stack and frame pointer
// names differ between the 32-bit and the 64-bit mode, which is the whole reason
// a pass cannot hard-code one spelling.
func (p Profile) Frame() core.FrameModel {
	return core.FrameModel{
		StackPointer: p.StackPointer,
		FramePointer: p.StackFrame,
		Flags:        p.Flags,
		CalleeSaved:  p.CalleeSaved,
		StackGrowth:  -1,
	}
}

// Profile64 is the AMD64 profile.
func Profile64() Profile {
	return Profile{
		Kind:               core.ArchAMD64,
		Name:               "amd64",
		Mode:               64,
		Bits:               64,
		StackPointer:       "rsp",
		StackFrame:         "rbp",
		InstructionPointer: "rip",
		Accumulator:        "rax",
		AccumulatorHigh:    "rdx",
		Accumulator8:       "al",
		Accumulator8High:   "ah",
		CountRegister:      "rcx",
		Flags:              "rflags",
		Endbr:              "endbr64",
		CalleeSaved:        []string{"rbp", "rbx", "r12", "r13", "r14", "r15"},
		Canary:             core.CanaryModel{Kind: core.CanarySegment, Register: "fs", Offset: 0x28},
	}
}

// Profile32 is the 386 profile.
func Profile32() Profile {
	return Profile{
		Kind:             core.Arch386,
		Name:             "386",
		Mode:             32,
		Bits:             32,
		StackPointer:     "esp",
		StackFrame:       "ebp",
		Accumulator:      "eax",
		AccumulatorHigh:  "edx",
		Accumulator8:     "al",
		Accumulator8High: "ah",
		CountRegister:    "ecx",
		Flags:            "eflags",
		CalleeSaved:      []string{"ebp", "ebx", "esi", "edi"},
	}
}

// Profile16 is the 8086 profile: real mode, sixteen-bit addresses and no
// extended registers. The MZ and COM loaders select it.
func Profile16() Profile {
	return Profile{
		Kind:             core.Arch8086,
		Name:             "8086",
		Mode:             16,
		Bits:             16,
		StackPointer:     "sp",
		StackFrame:       "bp",
		Accumulator:      "ax",
		AccumulatorHigh:  "dx",
		Accumulator8:     "al",
		Accumulator8High: "ah",
		CountRegister:    "cx",
		Flags:            "flags",
		CalleeSaved:      []string{"bp", "bx", "si", "di"},
	}
}

// stackSlot is the width of one pushed slot: four bytes in 32-bit mode and
// eight in 64-bit mode.
func (p Profile) stackSlot() uint64 { return uint64(p.Bits / 8) }

// Check validates one target against this profile.
func (p Profile) Check(target core.Target) error {
	if target.Architecture != p.Kind {
		return fmt.Errorf("wanted architecture %s, got %s", p.Kind, target.Architecture)
	}
	if target.Bits != int(p.Bits) {
		return fmt.Errorf("%s requires %d-bit addresses, got %d", p.Name, p.Bits, target.Bits)
	}
	if target.Mode != "" {
		return fmt.Errorf("%s does not support execution mode %q", p.Name, target.Mode)
	}
	return nil
}

// accumulatorNames returns the register pair used by MUL, DIV and CDQ-style
// sign extension at one width.
func (p Profile) accumulatorNames(bits uint16) (string, string) {
	switch bits {
	case 8:
		return p.Accumulator8, p.Accumulator8High
	case 16:
		return accumulator16(p), accumulatorHigh16(p)
	case 32:
		return accumulator32(p), accumulatorHigh32(p)
	default:
		return p.Accumulator, p.AccumulatorHigh
	}
}

func accumulator16(p Profile) string {
	if p.Mode == 32 {
		return "ax"
	}
	return "ax"
}

func accumulatorHigh16(p Profile) string {
	return "dx"
}

func accumulator32(p Profile) string {
	if p.Mode == 32 {
		return "eax"
	}
	return "eax"
}

func accumulatorHigh32(p Profile) string {
	return "edx"
}

// register renders one decoded register as a stable name and width.
func (p Profile) register(reg x86asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= x86asm.AL && reg <= x86asm.R15B:
		return strings.ToLower(reg.String()), 8, true
	case reg >= x86asm.AX && reg <= x86asm.R15W:
		return strings.ToLower(reg.String()), 16, true
	case reg >= x86asm.EAX && reg <= x86asm.R15L:
		return strings.ToLower(reg.String()), 32, true
	case reg >= x86asm.RAX && reg <= x86asm.R15:
		return strings.ToLower(reg.String()), 64, true
	case reg == x86asm.RIP:
		if p.InstructionPointer == "" {
			return "", 0, false
		}
		return p.InstructionPointer, 64, true
	case reg >= x86asm.X0 && reg <= x86asm.X15:
		return fmt.Sprintf("xmm%d", int(reg-x86asm.X0)), 128, true
	default:
		return "", 0, false
	}
}
