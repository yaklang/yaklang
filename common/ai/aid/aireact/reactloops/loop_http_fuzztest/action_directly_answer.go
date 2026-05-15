package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopActionDirectlyAnswerHTTPFuzztest = &reactloops.LoopAction{
	ActionType:  "directly_answer",
	Description: "用于回答 HTTP 安全测试过程中的阶段性结论或简短问题。短答案可用 answer_payload；需要 Markdown 分段、列表、表格或更复杂展示时，使用 FINAL_ANSWER AITAG。若已验证漏洞或存在应落库风险，必须先调用 generate_risk 保存 Risk；回答漏洞结论时必须包含证据链。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(`仅在回答简短测试过程问答时使用。若答案较长、包含多段 Markdown、列表、表格或复杂结构，请留空此字段并改用 <|FINAL_ANSWER_...|> 标签。answer_payload 与 <|FINAL_ANSWER_...|> 互斥，不要同时使用。不要把 fuzz、改包、整包生成或 generate_risk 动作伪装成 directly_answer。已验证漏洞时必须先保存 Risk，并在答案中包含证据链。`),
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
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := strings.TrimSpace(action.GetString("answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("answer_payload"))
		}
		tagPayload := strings.TrimSpace(action.GetString("tag_final_answer"))
		if tagPayload == "" {
			tagPayload = strings.TrimSpace(loop.Get("tag_final_answer"))
		}
		if payload != "" && tagPayload != "" {
			return utils.Error("directly_answer requires exactly one of answer_payload or FINAL_ANSWER tag, but both were provided")
		}
		if payload == "" {
			payload = tagPayload
		}
		if payload == "" {
			// 用 WrapDirectlyAnswerError 升级为带 nonce 的 AITAG 提示, 让 AI 重试时能
			// 用 FINAL_ANSWER tag 自纠正, 避免 5 次重试黑洞 + fatal abort.
			// 关键词: directly_answer ActionVerifier AITAG hint, 5 次重试黑洞修复
			return reactloops.WrapDirectlyAnswerError(loop, utils.Error("directly_answer requires answer_payload or FINAL_ANSWER tag, but both are empty"))
		}
		if loopHTTPFuzzDirectAnswerRequiresSavedRisk(loop, payload) {
			return utils.Error("directly_answer blocked: current fuzz result contains verified or defensible vulnerability evidence, but no Risk has been saved yet. Call generate_risk first, include every independent risk in the risks array when there are multiple findings, then answer with an evidence chain.")
		}
		if loopHTTPFuzzHasRiskWorthyEvidence(loop, payload) && !loopHTTPFuzzAnswerHasEvidenceChain(payload) {
			return utils.Error("directly_answer blocked: verified vulnerability conclusions must include an evidence chain, such as tested payloads/parameters, representative request or HTTPFlow, response differences, and reproduction steps.")
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := strings.TrimSpace(loop.Get("directly_answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(loop.Get("tag_final_answer"))
		}
		if payload == "" {
			operator.Fail("directly_answer action must have 'answer_payload' field")
			return
		}

		recordLoopHTTPFuzzMetaAction(
			loop,
			"directly_answer",
			"回答当前测试过程问题或总结当前阶段进展",
			utils.ShrinkTextBlock(payload, 240),
		)
		markLoopHTTPFuzzDirectlyAnswered(loop)
		persistLoopHTTPFuzzSessionContext(loop, "directly_answer")
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)

		var timeline strings.Builder
		timeline.WriteString("user input:\n")
		timeline.WriteString(utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "))
		timeline.WriteString("\nai directly answer:\n")
		timeline.WriteString(utils.PrefixLines(payload, "  | "))
		if currentSummary := getCurrentRequestSummary(loop); currentSummary != "" {
			timeline.WriteString("\ncurrent request summary:\n")
			timeline.WriteString(utils.PrefixLines(currentSummary, "  = "))
		}
		if changeSummary := strings.TrimSpace(loop.Get("request_change_summary")); changeSummary != "" {
			timeline.WriteString("\nlatest merge summary:\n")
			timeline.WriteString(utils.PrefixLines(utils.ShrinkTextBlock(changeSummary, 800), "  ~ "))
		}
		if decision := strings.TrimSpace(loop.Get("request_review_decision")); decision != "" {
			timeline.WriteString(fmt.Sprintf("\nrequest review decision: %s", decision))
		}
		invoker.AddToTimeline("directly_answer", timeline.String())
		operator.Exit()
	},
}

func loopHTTPFuzzDirectAnswerRequiresSavedRisk(loop *reactloops.ReActLoop, payload string) bool {
	if loopHTTPFuzzHasSavedRisk(loop) {
		return false
	}
	return loopHTTPFuzzHasRiskWorthyEvidence(loop, payload)
}

func loopHTTPFuzzHasSavedRisk(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	return strings.TrimSpace(loop.Get("generated_risk_ids")) != "" ||
		strings.TrimSpace(loop.Get("generated_risk_id")) != ""
}

func loopHTTPFuzzHasRiskWorthyEvidence(loop *reactloops.ReActLoop, payload string) bool {
	verification := ""
	analysis := ""
	if loop != nil {
		verification = strings.TrimSpace(loop.Get("verification_result"))
		analysis = strings.TrimSpace(firstNonEmptyString(
			loop.Get("diff_result_analysis"),
			loop.Get("diff_result_compressed"),
			loop.Get("diff_result"),
		))
	}
	if strings.Contains(strings.ToLower(verification), "satisfied: true") {
		return true
	}
	combined := strings.TrimSpace(analysis + "\n" + payload)
	if combined == "" {
		return false
	}
	lower := strings.ToLower(combined)
	negativeSignals := []string{"未发现", "暂无", "暂未", "没有发现", "证据不足", "不成立", "未达到", "false positive"}
	for _, signal := range negativeSignals {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return false
		}
	}
	positiveSignals := []string{
		"漏洞成立", "已验证", "确认存在", "稳定复现", "可复现",
		"sql 注入", "sql injection", "xss", "idor", "越权", "权限校验缺失",
		"未授权访问", "信息泄漏", "敏感信息", "路径穿越", "ssrf", "命令执行",
	}
	for _, signal := range positiveSignals {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

func loopHTTPFuzzAnswerHasEvidenceChain(payload string) bool {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return false
	}
	lower := strings.ToLower(payload)
	evidenceMarkers := []string{"证据链", "证据", "触发依据", "复现", "代表性", "httpflow"}
	technicalMarkers := []string{"payload", "参数", "请求", "响应", "状态码", "响应长度", "httpflow", "数据包"}
	hasEvidence := false
	for _, marker := range evidenceMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			hasEvidence = true
			break
		}
	}
	if !hasEvidence {
		return false
	}
	for _, marker := range technicalMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
