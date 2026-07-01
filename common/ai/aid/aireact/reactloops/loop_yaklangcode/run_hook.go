package loop_yaklangcode

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// buildYaklangPostSyntaxCleanRunHook runs YAK_MAIN self-test after static lint passes.
func buildYaklangPostSyntaxCleanRunHook(r aicommon.AIInvokeRuntime) loopinfra.PostSyntaxCleanHook {
	cfg := r.GetConfig()
	return func(loop *reactloops.ReActLoop, op *reactloops.LoopActionHandlerOperator) (string, bool) {
		if loop == nil || op == nil {
			return "", false
		}
		code := strings.TrimSpace(loop.Get("full_code"))
		policy := ClassifyYakScriptRunPolicy(code)

		if policy.BlockExitNoSelfTest {
			feedback := FormatMissingSelfTestFeedback(policy)
			loop.Set(loopVarYakRunOK, "false")
			loop.Set(loopVarYakRunLastFeedback, feedback)
			r.AddToTimeline("run_skipped_need_self_test", utils.ShrinkTextBlock(feedback, 512))
			emitYaklangRunNeedSelfTest(loop, policy)
			log.Warnf("yaklang self-test blocked: %s kind=%s", policy.SkipReason, policy.Kind)
			return feedback, true
		}

		if !policy.ShouldExecuteRun {
			if policy.SkipReason != "" {
				emitYaklangRunSkipped(loop, policy.SkipReason)
				r.AddToTimeline("run_skipped", policy.SkipReason)
				logYakRunSelfTestSkip(policy.SkipReason)
			}
			return "", false
		}

		if utils.InTestcase() {
			logYakRunSelfTestSkip("testcase mode")
			return "", false
		}
		if yakRunSelfTestDisabled(cfg) {
			logYakRunSelfTestSkip("disabled by config")
			return "", false
		}

		task := loop.GetCurrentTask()
		if task == nil && op != nil {
			task = op.GetTask()
		}
		if task == nil {
			log.Warnf("skip yaklang self-test: no current task")
			return "", false
		}

		attempts := loop.GetInt(loopVarYakRunAttempts) + 1
		loop.Set(loopVarYakRunAttempts, fmt.Sprint(attempts))
		maxRetries := yakRunSelfTestMaxRetries(cfg)
		if attempts > maxRetries {
			log.Warnf("yaklang self-test exceeded max retries (%d), allowing loop exit", maxRetries)
			loop.Set(loopVarYakRunOK, "false")
			return "", false
		}

		absPath := resolveYakRunAbsPath(loop.Get("editor_file_path"), loop.Get("filename"))
		timeoutSec := yakRunSelfTestTimeoutSec(cfg)
		emitYaklangRunStart(loop, absPath, policy)

		result, err := RunYakSelfTest(task.GetContext(), code, absPath, timeoutSec)
		emitYaklangRunFinish(loop, absPath, result, err)
		if err == nil {
			loop.Set(loopVarYakRunOK, "true")
			loop.Set(loopVarYakRunOutput, result.Output)
			loop.Set(loopVarYakRunLastFeedback, "")
			loop.Set(loopVarYakRunAttempts, "0")
			msg := fmt.Sprintf("YAK_MAIN self-test passed (%d bytes log)", len(result.Output))
			r.AddToTimeline("run_passed", msg)
			log.Infof("yaklang self-test passed: path=%s log_bytes=%d", absPath, len(result.Output))
			return "", false
		}

		feedback := FormatRunFailureForAI(result, err)
		loop.Set(loopVarYakRunOK, "false")
		loop.Set(loopVarYakRunOutput, result.Output)
		loop.Set(loopVarYakRunLastFeedback, feedback)
		r.AddToTimeline("run_failed", utils.ShrinkTextBlock(feedback, 512))
		log.Warnf("yaklang self-test failed (attempt %d/%d): %v", attempts, maxRetries, err)
		return feedback, true
	}
}
