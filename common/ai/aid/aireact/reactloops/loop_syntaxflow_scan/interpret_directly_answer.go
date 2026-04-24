package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	minDirectAnswerWhenFinalReport = 2000
	maxDirectAnswerFirstIterShort  = 500
)

// loopActionDirectlyAnswerSyntaxflowScan 解读子环的 directly_answer：防止首轮不调用工具就短答结束；终局时要求足够篇幅。
var loopActionDirectlyAnswerSyntaxflowScan = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "Directly answer; for final merged report when sf_scan_final_report_due=1 use a long body or <|FINAL_ANSWER|> tag. Before tools on iter 0, do not use only a short status line.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description("Short payload only if not final report; for long Markdown leave empty and use <|FINAL_ANSWER|> tag. Mutually exclusive with FINAL_ANSWER tag."),
		),
	},
	AITagStreamFields: []*reactloops.LoopAITagField{
		{
			TagName:      "FINAL_ANSWER",
			VariableName: "tag_final_answer",
			AINodeId:     "re-act-loop-answer-payload",
			ContentType:  aicommon.TypeTextMarkdown,
		},
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName:   "answer_payload",
			AINodeId:    "re-act-loop-answer-payload",
			ContentType: aicommon.TypeTextMarkdown,
		},
	},
	ActionVerifier: directlyAnswerSyntaxflowScanVerifier,
	ActionHandler:  directlyAnswerSyntaxflowScanHandler,
}

func directAnswerPayloadText(loop *reactloops.ReActLoop, action *aicommon.Action) string {
	payload := action.GetString("answer_payload")
	if payload == "" {
		payload = action.GetInvokeParams("next_action").GetString("answer_payload")
	}
	if payload == "" {
		return strings.TrimSpace(loop.Get("tag_final_answer"))
	}
	return strings.TrimSpace(payload)
}

func directlyAnswerSyntaxflowScanVerifier(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	payload := directAnswerPayloadText(loop, action)
	if payload == "" {
		return utils.Error("answer_payload or FINAL_ANSWER tag is required for directly_answer but both are empty")
	}
	iter := loop.GetCurrentIterationIndex()
	finalDue := strings.TrimSpace(loop.Get(sfu.LoopVarSFFinalReportDue)) == "1"
	if finalDue && len([]rune(payload)) < minDirectAnswerWhenFinalReport {
		return utils.Errorf("sf_scan_final_report_due=1 时终局大报告须至少 %d 字（当前 %d）。请用 reload_* 取全量后，用 Markdown 分节长文或 <|FINAL_ANSWER|> 流式输出完整报告。",
			minDirectAnswerWhenFinalReport, len([]rune(payload)))
	}
	if !finalDue && iter == 0 {
		used := strings.TrimSpace(loop.Get("sf_interpret_tool_used")) == "1"
		if !used && len([]rune(payload)) < maxDirectAnswerFirstIterShort {
			return utils.Error("首轮过短且尚未调用 reload_syntaxflow_scan_session / reload_ssa_risk_overview / set_ssa_risk_review_target 之一。请先使用工具以 DB 数据为准，或输出长文（约 ≥500 字）。")
		}
	}
	loop.Set("directly_answer_payload", payload)
	return nil
}

func directlyAnswerSyntaxflowScanHandler(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	invoker := loop.GetInvoker()
	payload := loop.Get("directly_answer_payload")
	if payload == "" {
		payload = strings.TrimSpace(loop.Get("tag_final_answer"))
	}
	if payload == "" {
		operator.Fail("directly_answer: empty payload")
		return
	}
	invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
	invoker.EmitResultAfterStream(payload)
	invoker.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n%s\nai directly answer:\n%v",
		utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
		utils.PrefixLines(payload, "  | "),
	))
	operator.Exit()
}
