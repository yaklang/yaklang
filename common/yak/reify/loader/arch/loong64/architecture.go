// Package loong64 parses LoongArch 64-bit machine code into LowIR.
// Instructions are four bytes wide and the architecture is little-endian.
package loong64

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

const (
	// architecture is the only architecture this package decodes.
	architecture = core.ArchLOONG64
	// architectureName is the short name used in diagnostics.
	architectureName = "loong64"
	// addressBits is the integer and address width.
	addressBits uint16 = 64
)

// checkTarget validates one target against this architecture.
func checkTarget(target core.Target) error {
	if target.Architecture != architecture {
		return fmt.Errorf("wanted architecture %s, got %s", architecture, target.Architecture)
	}
	if target.Bits != int(addressBits) {
		return fmt.Errorf("%s requires %d-bit addresses, got %d", architectureName, addressBits, target.Bits)
	}
	if target.Mode != "" {
		return fmt.Errorf("%s does not support execution mode %q", architectureName, target.Mode)
	}
	if target.ByteOrder != core.ByteOrderUnknown && target.ByteOrder != core.LittleEndian {
		return fmt.Errorf("%s is little-endian, got %s", architectureName, target.ByteOrder)
	}
	return nil
}
