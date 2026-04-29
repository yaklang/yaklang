package loop_vuln_verify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// Verdict values used in the final report.
const (
	verdictConfirmed    = "CONFIRMED"
	verdictNotConfirmed = "NOT_CONFIRMED"
	verdictInconclusive = "INCONCLUSIVE"
)

// loopActionConclude overrides the default directly_answer action.
// The FINAL_REPORT tag is the preferred output channel; if the AI omits it
// (e.g. due to context pressure), the handler constructs a report from the
// already-collected evidence so the verifier never blocks progress.
var loopActionConclude = &reactloops.LoopAction{
	ActionType: "directly_answer",
	Description: "输出最终漏洞验证报告。" +
		"提供结论（CONFIRMED / NOT_CONFIRMED / INCONCLUSIVE），" +
		"并将完整的 Markdown 报告放入 FINAL_REPORT 标签内。" +
		"在至少完成阶段 1（可复现性评估）之前不得调用本动作。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("verdict",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("最终结论：CONFIRMED | NOT_CONFIRMED | INCONCLUSIVE"),
		),
		aitool.WithStringParam("human_readable_thought",
			aitool.WithParam_Description("简要说明选择该结论的理由"),
		),
	},
	AITagStreamFields: []*reactloops.LoopAITagField{
		{
			TagName:      "FINAL_REPORT",
			VariableName: keyFinalVerdict,
			AINodeId:     "re-act-loop-answer-payload",
			ContentType:  aicommon.TypeTextMarkdown,
		},
	},
	// Verifier only checks the verdict enum value.
	// FINAL_REPORT tag content is optional — the handler synthesises a report
	// from collected evidence when the tag is absent.
	ActionVerifier: func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
		verdict := action.GetString("verdict")
		switch verdict {
		case verdictConfirmed, verdictNotConfirmed, verdictInconclusive:
			return nil
		default:
			return utils.Errorf("verdict must be CONFIRMED | NOT_CONFIRMED | INCONCLUSIVE, got %q", verdict)
		}
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		verdict := action.GetString("verdict")
		thought := action.GetString("human_readable_thought")
		invoker := loop.GetInvoker()

		// Use tag content when available; fall back to synthesised report.
		report := loop.Get(keyFinalVerdict)
		if report == "" {
			report = buildFallbackReportFromState(loop, verdict, thought)
		}

		loop.Set(keyVerdictDelivered, "true")

		invoker.AddToTimeline("vuln_verify_concluded",
			fmt.Sprintf("verdict=%s evidence_count=%s", verdict, loop.Get(keyEvidenceCount)))

		invoker.EmitFileArtifactWithExt("vuln_verify_report", ".md", report)
		invoker.EmitResultAfterStream(report)
		op.Exit()
	},
}

// buildFallbackReportFromState constructs a minimal verification report from the
// loop state when the AI omits the FINAL_REPORT tag.
func buildFallbackReportFromState(loop *reactloops.ReActLoop, verdict, thought string) string {
	var sb strings.Builder

	verdictLabel := map[string]string{
		verdictConfirmed:    "已确认（CONFIRMED）",
		verdictNotConfirmed: "未确认（NOT_CONFIRMED）",
		verdictInconclusive: "结论不确定（INCONCLUSIVE）",
	}[verdict]
	if verdictLabel == "" {
		verdictLabel = verdict
	}

	sb.WriteString("# 漏洞验证报告\n\n")
	sb.WriteString(fmt.Sprintf("## 结论：%s\n\n", verdictLabel))

	if finding := loop.Get(keyFindingDescription); finding != "" {
		sb.WriteString(fmt.Sprintf("**发现内容**：%s\n\n", utils.ShrinkTextBlock(finding, 300)))
	}
	if target := loop.Get(keyTargetInfo); target != "" && target != "not_provided" {
		sb.WriteString(fmt.Sprintf("**目标**：%s\n\n", target))
	}
	if thought != "" {
		sb.WriteString(fmt.Sprintf("**摘要**：%s\n\n", thought))
	}

	// Append collected evidence.
	evidenceJSON := loop.Get(keyEvidenceJSON)
	if evidenceJSON != "" && evidenceJSON != "[]" {
		var entries []evidenceEntry
		if err := json.Unmarshal([]byte(evidenceJSON), &entries); err == nil && len(entries) > 0 {
			sb.WriteString("## 证据\n\n")
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("### #%d — %s（%s）\n\n", e.Seq, e.Type, e.Significance))
				sb.WriteString(e.Observation + "\n\n")
				if e.RawData != "" {
					sb.WriteString("```\n" + utils.ShrinkTextBlock(e.RawData, 400) + "\n```\n\n")
				}
			}
		}
	}

	return strings.TrimSpace(sb.String())
}
