package loop_infosec_recon

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

const (
	jsStaticPathFailThreshold = 3
	spinRecoveryHintTemplate  = `[infosec_recon SPIN recovery] js_static_extract_ai failed %d times due to path/comma issues. ` +
		`Use js_static_extract_ai with dir=%q (NOT paths=). Alternatively use grep_text on individual .js files, then api_pool_merge. ` +
		`Do NOT repeat the same paths= value.`
)

func infosecRecordJsStaticPathFailure(loop *reactloops.ReActLoop, feedback string) {
	if !strings.Contains(strings.ToLower(feedback), "path") &&
		!strings.Contains(feedback, "no paths") {
		return
	}
	n := infosecJsStaticPathFailCount(loop) + 1
	loop.Set(keyJsStaticPathFailCount, strconv.Itoa(n))
	if n >= jsStaticPathFailThreshold {
		infosecRefreshSpinRecoveryHint(loop)
	}
}

func infosecClearJsStaticPathFailures(loop *reactloops.ReActLoop) {
	loop.Set(keyJsStaticPathFailCount, "0")
	loop.Set(keySpinRecoveryHint, "")
}

func infosecJsStaticPathFailCount(loop *reactloops.ReActLoop) int {
	n, _ := strconv.Atoi(strings.TrimSpace(loop.Get(keyJsStaticPathFailCount)))
	if n < 0 {
		return 0
	}
	return n
}

func infosecRefreshSpinRecoveryHint(loop *reactloops.ReActLoop) {
	dir := strings.TrimSpace(loop.Get(keyVerifiedJsDir))
	if dir == "" {
		return
	}
	n := infosecJsStaticPathFailCount(loop)
	hint := fmt.Sprintf(spinRecoveryHintTemplate, n, dir)
	loop.Set(keySpinRecoveryHint, hint)
	log.Warnf("infosec_recon: %s", hint)
}

func buildInfosecPostIterationHook(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, _ aicommon.AIStatefulTask, isDone bool, reason any, _ *reactloops.OnPostIterationOperator) {
		if isDone {
			return
		}
		if loop.ShouldForceExitDueToSpin() {
			dir := strings.TrimSpace(loop.Get(keyVerifiedJsDir))
			if dir != "" {
				infosecRefreshSpinRecoveryHint(loop)
				r.AddToTimeline("infosec_spin_recovery",
					fmt.Sprintf("SPIN detected at iteration %d; use js_static_extract_ai dir=%s instead of paths=", iteration, dir))
			}
		}
		_ = reason
	})
}
