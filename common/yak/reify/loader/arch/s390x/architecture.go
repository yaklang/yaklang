// Package s390x parses IBM z/Architecture machine code into LowIR.
// Instructions are two, four or six bytes long and the architecture is
// big-endian, so the decoder needs the instruction length before it can name the
// operation.
package s390x

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

const (
	// architecture is the only architecture this package decodes.
	architecture = core.ArchS390X
	// architectureName is the short name used in diagnostics.
	architectureName = "s390x"
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
	if target.ByteOrder != core.ByteOrderUnknown && target.ByteOrder != core.BigEndian {
		return fmt.Errorf("%s is big-endian, got %s", architectureName, target.ByteOrder)
	}
	return nil
}
