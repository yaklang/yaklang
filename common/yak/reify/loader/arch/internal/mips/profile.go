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
