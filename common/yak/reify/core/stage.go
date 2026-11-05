package core

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// SSAIRBuilder converts LowIR into a YakSSA program.
type SSAIRBuilder interface {
	BuildSSAIR(context.Context, *LowIR) (*ssa.Program, error)
}

// SourceRenderer renders a YakSSA program as source-like text.
type SourceRenderer interface {
	RenderSource(context.Context, *ssa.Program) (string, error)
}
