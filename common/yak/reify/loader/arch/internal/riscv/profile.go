// Package riscv implements the RISC-V machine-code parser shared by the
// RISCV32 and RISCV64 architectures. The instruction encoding is the same for
// both widths; only the default register width and the load/store widths differ.
package riscv

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Profile is the per-width difference between the two RISC-V parsers.
type Profile struct {
	// Kind is the architecture this profile decodes.
	Kind core.Architecture
	// Name is the short architecture name used in diagnostics.
	Name string
	// Bits is the default integer and address width.
	Bits uint16
}

// Profile64 is the RV64 profile.
func Profile64() Profile {
	return Profile{Kind: core.ArchRISCV64, Name: "riscv64", Bits: 64}
}

// Profile32 is the RV32 profile.
func Profile32() Profile {
	return Profile{Kind: core.ArchRISCV32, Name: "riscv32", Bits: 32}
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
