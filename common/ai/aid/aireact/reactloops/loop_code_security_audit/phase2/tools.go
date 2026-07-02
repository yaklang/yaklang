package phase2

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

const phase2FSToolOwnerTag = "CodeAuditPhase2"

// phase2WhitelistFSTools are the only builtin tools exposed in per-category Phase 2 scan loops.
// Discovery expansion uses fast_context (custom action) or these FS tools in phase A;
// phase B uses read_file + content-mode grep (enforced by guards in phase2_grep_guard.go).
// Generic require_tool / directly_call_tool are disabled (WithAllowToolCall(false)).
var phase2WhitelistFSTools = []string{
	"grep",
	"read_file",
	"find_file",
}

func buildPhase2WhitelistFSToolOptions(r aicommon.AIInvokeRuntime) []reactloops.ReActLoopOption {
	opts := make([]reactloops.ReActLoopOption, 0, len(phase2WhitelistFSTools))
	for _, toolName := range phase2WhitelistFSTools {
		opt := loopinfra.RegisterBuiltinFSToolLoopAction(r, phase2FSToolOwnerTag, toolName, nil)
		opts = append(opts, opt)
	}
	return opts
}
