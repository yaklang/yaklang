// Package arch provides one parser per instruction-set architecture. Every
// architecture in All decodes machine code into LowIR.
package arch

import (
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/amd64"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/arm"
	arm64impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/arm64"
	loong64impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/loong64"
	mipsimpl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/mips"
	mips64impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/mips64"
	ppcimpl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/ppc"
	ppc64impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/ppc64"
	riscv32impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/riscv32"
	riscv64impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/riscv64"
	s390ximpl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/s390x"
	x86impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/x86"
	x8616impl "github.com/yaklang/yaklang/common/yak/reify/loader/arch/x86_16"
)

var ErrDecoderUnavailable = errors.New("instruction decoder is not implemented")

// All returns one registered parser per architecture.
func All() []core.Parser {
	return []core.Parser{
		amd64.Parser{}, x86impl.Parser{}, arm64impl.Parser{}, arm.Parser{},
		riscv64impl.Parser{}, riscv32impl.Parser{},
		ppc64impl.Parser{}, ppcimpl.Parser{},
		mips64impl.Parser{}, mipsimpl.Parser{},
		s390ximpl.Parser{}, loong64impl.Parser{}, x8616impl.Parser{},
	}
}

type rule struct {
	bits  int
	modes []string
}

func checkTarget(target core.Target, kind core.Architecture, spec rule) error {
	if target.Architecture != kind {
		return fmt.Errorf("wanted architecture %s, got %s", kind, target.Architecture)
	}
	if target.Bits != spec.bits {
		return fmt.Errorf("%s requires %d-bit addresses, got %d", kind, spec.bits, target.Bits)
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
