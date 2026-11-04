// Package arch provides one parser per instruction-set architecture. AMD64
// decodes machine code into LowIR. The other architectures register the
// dispatch contract and validate targets; their Parse reports
// ErrDecoderUnavailable.
package arch

import (
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

var ErrDecoderUnavailable = errors.New("instruction decoder is not implemented")

// All returns one registered parser per architecture.
func All() []core.Parser {
	return []core.Parser{
		AMD64{}, X86{}, ARM{}, ARM64{}, RISCV32{}, RISCV64{}, MIPS{}, MIPS64{}, PPC{}, PPC64{},
	}
}

type rule struct {
	bits       int
	littleOnly bool
	modes      []string
}

func checkTarget(target core.Target, kind core.Architecture, spec rule) error {
	if target.Architecture != kind {
		return fmt.Errorf("wanted architecture %s, got %s", kind, target.Architecture)
	}
	if target.Bits != spec.bits {
		return fmt.Errorf("%s requires %d-bit addresses, got %d", kind, spec.bits, target.Bits)
	}
	if spec.littleOnly && target.ByteOrder != core.LittleEndian {
		return fmt.Errorf("%s requires little-endian byte order", kind)
	}
	if target.Mode != "" {
		for _, mode := range spec.modes {
			if target.Mode == mode {
				return nil
			}
		}
		return fmt.Errorf("%s does not support execution mode %q", kind, target.Mode)
	}
	return nil
}

func unavailable(kind core.Architecture) (*core.ParseResult, error) {
	return nil, fmt.Errorf("%w: %s", ErrDecoderUnavailable, kind)
}
