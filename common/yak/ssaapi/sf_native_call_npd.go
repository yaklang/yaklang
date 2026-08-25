package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
)

// NativeCall_NPD finds null-pointer dereference sites (independent of UAF).
// Usage:
//   - *<npd()> as $npd                      // all NPD sites
//   - $ptr<npd()> as $npd                   // NPD related to $ptr
//   - <npd(target=$ptr)> as $npd            // same, named target (receiver may be *)
const NativeCall_NPD = "npd"

func nativeCallNPD(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeNativeCall(vs, frame, params, "npd",
		func(prog *ssa.Program, seeds []ssa.Value, full bool) []*lifetime.Finding {
			if full {
				return lifetime.FindNPDUses(prog)
			}
			return lifetime.FindNPDUsesRelated(prog, seeds)
		},
		func(kind string) bool {
			return kind == lifetime.KindNPD
		},
	)
}
