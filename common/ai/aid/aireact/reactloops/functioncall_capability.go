package reactloops

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// resolveFunctionCallMode chooses the transport for this task before the first
// loop prompt is assembled. Explicit loop options retain their old force-on or
// force-off behavior; only the config default is auto-detected.
func (r *ReActLoop) resolveFunctionCallMode(ctx context.Context) {
	if r == nil || r.functionCallModeExplicit || !r.functionCallModeRequested {
		return
	}
	config, ok := r.config.(*aicommon.Config)
	if !ok {
		// Custom runtimes retain their explicit configuration contract.
		return
	}
	capability, err := config.CheckToolCallCapability(ctx, r.useSpeedPriorityAI)
	r.functionCallMode = capability == aicommon.ToolCallSupported
	if err != nil {
		log.Warnf("ReActLoop[%s] tool-call capability probe: %v; using text actions", r.loopName, err)
		return
	}
	log.Infof("ReActLoop[%s] tool-call capability=%s, native mode=%t", r.loopName, capability, r.functionCallMode)
}
