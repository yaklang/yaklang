// Package ppc implements the PowerPC machine-code parser shared by the PPC and
// PPC64 architectures. Both widths share the encoding; the register width and
// the available 64-bit instructions differ.
package ppc

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Profile is the per-width difference between the two PowerPC parsers.
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

// Profile64 is the ppc64 profile.
func Profile64() Profile {
	return Profile{Kind: core.ArchPPC64, Name: "ppc64", Bits: 64}
}

// Profile32 is the ppc profile.
func Profile32() Profile {
	return Profile{Kind: core.ArchPPC, Name: "ppc", Bits: 32}
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

// Frame describes the ELF ABI call frame. r1 is the stack pointer, r31 the frame
// pointer and the link register holds the return address.
func (p Profile) Frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "r1",
		FramePointer:  "r31",
		ReturnAddress: "lr",
		CalleeSaved:   []string{"r14", "r15", "r16", "r17", "r18", "r19", "r20", "r21", "r22", "r23", "r24", "r25", "r26", "r27", "r28", "r29", "r30", "r31"},
		StackGrowth:   -1,
	}
}

// Registers describes register naming and the ELF ABI argument registers.
func (p Profile) Registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:    core.RegisterNamingPlain,
		Width:     p.Bits,
		Flags:     "",
		Arguments: []string{"r3", "r4", "r5", "r6", "r7", "r8", "r9", "r10"},
		Return:    "r3",
	}
}
