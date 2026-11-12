// Package s390x implements the IBM z/Architecture parser. Instructions are two,
// four or six bytes long and the architecture is big-endian, so the decoder
// needs the instruction length before it can name the operation.
package s390x

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Profile describes the one s390x variant.
type Profile struct {
	// Kind is the architecture this profile decodes.
	Kind core.Architecture
	// Name is the short architecture name used in diagnostics.
	Name string
	// Bits is the integer and address width.
	Bits uint16
}

// Profile64 is the s390x profile.
func Profile64() Profile {
	return Profile{Kind: core.ArchS390X, Name: "s390x", Bits: 64}
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
	if target.ByteOrder != core.ByteOrderUnknown && target.ByteOrder != core.BigEndian {
		return fmt.Errorf("%s is big-endian, got %s", p.Name, target.ByteOrder)
	}
	return nil
}
