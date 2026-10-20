package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// RegisterLightweightFocus keeps the ordinary assistant actions, initialization
// and completion behavior, adding only host-selected context options. Unlike a
// custom state machine, it needs no replacement actions or prompt templates.
func RegisterLightweightFocus(name string, preset []reactloops.ReActLoopOption, metadata ...reactloops.LoopMetadataOption) error {
	options := append([]reactloops.ReActLoopOption(nil), preset...)
	return reactloops.RegisterLoopFactory(name, func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
		combined := []reactloops.ReActLoopOption{
			reactloops.WithAllowAIForge(reactloops.IsPlanAndExecAllowed(nil, r)),
			reactloops.WithAllowPlanAndExec(reactloops.IsPlanAndExecAllowed(nil, r)),
		}
		combined = append(combined, options...)
		combined = append(combined, opts...)
		combined = append(combined, reactloops.WithAllowRAG(false))
		return NewLoop(name, r, combined...)
	}, metadata...)
}
