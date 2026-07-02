package phase3

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/log"
)

const autoVerifyGapReason = "Phase3 验证阶段未对该 finding 调用 conclude_finding（循环提前结束或跳过），系统自动标记为 uncertain，需人工复核。"

// FinalizeOnLoopEnd auto-fills remaining findings when Phase3 ends without complete_verify.
func FinalizeOnLoopEnd(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	verify *VerifyState,
	verifyCompleted bool,
	reason any,
) {
	if state == nil || verify == nil || verifyCompleted {
		return
	}
	if verify.AllDone() {
		log.Infof("[CodeAudit/Phase3] Verify loop ended without complete_verify but all findings concluded; continuing.")
		return
	}
	remaining := verify.RemainingIDs()
	if len(remaining) == 0 {
		return
	}
	reasonText := util.FormatLoopEndReason(reason)
	msg := fmt.Sprintf(
		"Phase3 循环结束前自动收尾：%d 个 finding 未 conclude_finding，已标记 uncertain。\n原因：%s\n未验证：\n%s",
		len(remaining), reasonText, strings.Join(remaining, ", "),
	)
	log.Warnf("[CodeAudit/Phase3] %s", msg)
	if r != nil {
		r.AddToTimeline("[VERIFY_AUTO_FINALIZE]", msg)
	}
	for _, id := range remaining {
		f := state.GetFindingByID(id)
		if f == nil {
			continue
		}
		state.UpsertVerifiedFinding(&model.VerifiedFinding{
			Finding:    f,
			Status:     model.VerifyUncertain,
			Confidence: clampConfidence(f.Confidence, 5),
			Reason:     autoVerifyGapReason + "（循环结束前自动收尾）",
			DataFlow:   f.DataFlow,
			Exploit:    f.ExploitScenario,
			Fix:        f.Recommendation,
		})
		verify.MarkConcluded(id)
	}
}

func clampConfidence(v, fallback int) int {
	if v < 1 {
		return fallback
	}
	if v > 10 {
		return 10
	}
	return v
}
