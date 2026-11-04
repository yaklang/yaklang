// Package rewriter reserves the typed YakSSA transformation boundary.
// No rewrite pass is implemented or enabled in the inspection-only CLI.
package rewriter

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Pass transforms an existing YakSSA program. Implementations must document
// their supported source semantics and preserve data/control/effect relations.
// Rewriting is not file loading, execution, or source-code printing.
type Pass interface {
	Name() string
	Rewrite(context.Context, *ssa.Program) error
}
