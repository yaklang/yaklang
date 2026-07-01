package loop_yaklangcode

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const yaklangNodeRunSelfTest = "yaklang-run-self-test"

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
	reactloops.EmitStatus(loop, "运行代码中 / Running code...")
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
		reactloops.EmitStatus(loop, "YAK_MAIN 自测失败，修复中 / Self-test failed, fixing...")
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
