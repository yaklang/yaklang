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

const yaklangNodeRunSelfTest = "yaklang-run-self-test"

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

		absPath := resolveYakRunAbsPath(loop.Get("editor_file_path"), loop.Get("filename"))
		timeoutSec := yakRunSelfTestTimeoutSec(cfg)
		emitYaklangRunStart(loop, absPath, policy)

		result, err := RunYakSelfTest(task.GetContext(), code, absPath, timeoutSec)
		emitYaklangRunFinish(loop, absPath, result, err)
		if err == nil {
			loop.Set(loopVarYakRunOK, "true")
			loop.Set(loopVarYakRunOutput, result.Output)
			loop.Set(loopVarYakRunLastFeedback, "")
			msg := fmt.Sprintf("YAK_MAIN self-test passed (%d bytes log)", len(result.Output))
			r.AddToTimeline("run_passed", msg)
			log.Infof("yaklang self-test passed: path=%s log_bytes=%d", absPath, len(result.Output))
			reactloops.EmitStatus(loop, "运行自测通过 / Self-test passed")
			return "", false
		}

		feedback := FormatRunFailureForAI(result, err)
		loop.Set(loopVarYakRunOK, "false")
		loop.Set(loopVarYakRunOutput, result.Output)
		loop.Set(loopVarYakRunLastFeedback, feedback)
		r.AddToTimeline("run_failed", utils.ShrinkTextBlock(feedback, 512))
		log.Warnf("yaklang self-test failed: %v", err)
		return feedback, true
	}
}

func emitYaklangRunStart(loop *reactloops.ReActLoop, absPath string, policy YakScriptRunPolicy) {
	if loop == nil {
		return
	}
	kind := string(policy.Kind)
	if kind == "" {
		kind = "unknown"
	}
	startLine := fmt.Sprintf(
		"运行 YAK_MAIN 自测: %s (类型: %s) / Running YAK_MAIN self-test: %s (kind: %s)",
		absPath, kind, absPath, kind,
	)
	reactloops.EmitActionLog(loop, yaklangNodeRunSelfTest, startLine)
	reactloops.EmitStatus(loop, "运行自测中 / Running self-test...")
}

func emitYaklangRunFinish(loop *reactloops.ReActLoop, absPath string, result YakRunResult, runErr error) {
	if loop == nil {
		return
	}
	var finishLine string
	var reference string
	if runErr == nil {
		finishLine = fmt.Sprintf(
			"自测通过: %s (%d bytes 日志) / Self-test passed: %s (%d bytes log)",
			absPath, len(result.Output), absPath, len(result.Output),
		)
	} else {
		errSummary := utils.ShrinkTextBlock(strings.TrimSpace(runErr.Error()), 256)
		finishLine = fmt.Sprintf(
			"自测失败: %s — %s / Self-test failed: %s — %s",
			absPath, errSummary, absPath, errSummary,
		)
	}
	if strings.TrimSpace(result.Output) != "" {
		body := result.Output
		if result.Truncated {
			body += "\n...(output truncated)"
		}
		_, reference = reactloops.SpillLongContent(loop, "yaklang_run_output", body)
	}
	reactloops.EmitActionLog(loop, yaklangNodeRunSelfTest, finishLine, reference)
	if runErr == nil {
		reactloops.EmitStatus(loop, "YAK_MAIN 自测通过 / Self-test passed")
	} else {
		reactloops.EmitStatus(loop, "运行自测失败，修复中 / Self-test failed, fixing...")
	}
}

func emitYaklangRunSkipped(loop *reactloops.ReActLoop, reason string) {
	if loop == nil || strings.TrimSpace(reason) == "" {
		return
	}
	line := fmt.Sprintf("跳过自测: %s / Skipped self-test: %s", reason, reason)
	reactloops.EmitActionLog(loop, yaklangNodeRunSelfTest, line)
	reactloops.EmitStatus(loop, FormatRunSkippedStatus(YakScriptRunPolicy{SkipReason: reason}))
}

func emitYaklangRunNeedSelfTest(loop *reactloops.ReActLoop, policy YakScriptRunPolicy) {
	if loop == nil {
		return
	}
	kind := string(policy.Kind)
	line := fmt.Sprintf(
		"需要 YAK_MAIN 自测块 (类型: %s) / YAK_MAIN self-test block required (kind: %s)",
		kind, kind,
	)
	reactloops.EmitActionLog(loop, yaklangNodeRunSelfTest, line)
	reactloops.EmitStatus(loop, "需要 YAK_MAIN 自测块 / YAK_MAIN self-test required")
}
