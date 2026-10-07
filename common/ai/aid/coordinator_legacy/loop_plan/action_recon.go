package loop_plan

import "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"

func appendReconResults(loop *reactloops.ReActLoop, content string) {
	old := loop.Get(PLAN_RECON_RESULTS_KEY)
	if old == "" {
		loop.Set(PLAN_RECON_RESULTS_KEY, content)
	} else {
		loop.Set(PLAN_RECON_RESULTS_KEY, old+"\n\n"+content)
	}
}
