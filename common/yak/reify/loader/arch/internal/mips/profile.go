// Package mips implements the MIPS machine-code parser shared by the MIPS and
// MIPS64 architectures. The encodings are identical; the register width and the
// word size differ.
package mips

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Profile is the per-width difference between the two MIPS parsers.
type Profile struct {
	// Kind is the architecture this profile decodes.
	Kind core.Architecture
	// Name is the short architecture name used in diagnostics.
	Name string
	// Bits is the default integer and address width.
	Bits uint16
	// Canary describes where the stack protector reads its guard. It is empty
	// for a target whose guard read this package does not recognize.
	Canary core.CanaryModel
}

// Profile64 is the mips64 profile.
func Profile64() Profile {
	return Profile{Kind: core.ArchMIPS64, Name: "mips64", Bits: 64}
}

// Profile32 is the mips profile.
func Profile32() Profile {
	return Profile{Kind: core.ArchMIPS, Name: "mips", Bits: 32}
}

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

// Frame describes the O32/N64 call frame. The stack pointer, the frame pointer
// and the return address register keep their architectural names.
func (p Profile) Frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "sp",
		FramePointer:  "fp",
		ReturnAddress: "ra",
		CalleeSaved:   []string{"s0", "s1", "s2", "s3", "s4", "s5", "s6", "s7", "fp"},
		StackGrowth:   -1,
	}
}

// Registers describes register naming and the O32/N64 argument registers.
func (p Profile) Registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:              core.RegisterNamingPlain,
		Width:               p.Bits,
		Arguments:           []string{"a0", "a1", "a2", "a3"},
		GlobalPointer:       "gp",
		GlobalPointerSymbol: "_gp",
		// A position-independent function receives its own address in t9 and
		// computes gp from it.
		EntryAddress: "t9",
		Return:       "v0",
	}
}
