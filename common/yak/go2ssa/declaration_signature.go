package go2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// registerGoDeclarationSignature publishes parameter/return/receiver metadata
// after all file declarations exist, but before any scheduled body can run.
// Building a signature does not drain a function body. The idempotent fallback
// covers callers that explicitly build one function outside the unit scheduler.
// Both callbacks belong to the declaring library: binding joins before body
// execution, and the frontend serializes bodies sharing this astbuilder.
func (b *astbuilder) registerGoDeclarationSignature(fun *ssa.Function, build func()) func() {
	ready := false
	prepare := func() {
		if ready {
			return
		}
		build()
		ready = true
	}
	lib := fun.GetProgram()
	lib.RegisterDeferredBuild(ssa.DeferredBuildKindBinding, fmt.Sprintf("go-signature:%s:%d", lib.Name, fun.GetId()), prepare)
	return prepare
}
