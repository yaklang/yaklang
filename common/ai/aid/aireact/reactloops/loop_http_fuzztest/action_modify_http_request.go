package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var modifyHTTPRequestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"modify_http_request",
		"修改当前生效的 HTTP 数据包，并把 merge 变化、审核结果和当前生效包明确展示给用户。",
		[]aitool.ToolOption{
			aitool.WithStringParam("modification_target", aitool.WithParam_Description("请用中文描述要修改哪个位置、希望变成什么样，例如“把 id 参数改成时间盲注探测载荷”。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("modification_reason", aitool.WithParam_Description("请用中文说明为什么要改这个数据包、怀疑的漏洞类型、以及必须遵守的安全边界。"), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("require_manual_review", aitool.WithParam_Description("是否要求在应用新数据包前先进行人工审核。")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "modification_target", AINodeId: "thought"},
			{FieldName: "modification_reason", AINodeId: "thought"},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(getCurrentRequestRaw(loop)) == "" {
				return fmt.Errorf("modify_http_request requires an existing current HTTP request; call set_http_request or restore session first")
			}
			if strings.TrimSpace(action.GetString("modification_target")) == "" {
				return fmt.Errorf("modification_target is required")
			}
			if strings.TrimSpace(action.GetString("modification_reason")) == "" {
				return fmt.Errorf("modification_reason is required")
			}
			packet := strings.TrimSpace(action.GetString(modifiedPacketContentField))
			if packet == "" {
				packet = strings.TrimSpace(loop.Get(modifiedPacketContentField))
			}
			if packet == "" {
				return fmt.Errorf("GEN_MODIFIED_PACKET AITAG is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			previousRequest := strings.TrimSpace(getCurrentRequestRaw(loop))
			modificationTarget := strings.TrimSpace(action.GetString("modification_target"))
			modificationReason := strings.TrimSpace(action.GetString("modification_reason"))
			requireManualReview := action.GetBool("require_manual_review")
			modifiedPacket := strings.TrimSpace(action.GetString(modifiedPacketContentField))
			if modifiedPacket == "" {
				modifiedPacket = strings.TrimSpace(loop.Get(modifiedPacketContentField))
			}
			if modifiedPacket == "" {
				operator.Fail(fmt.Errorf("modified packet content is empty"))
				return
			}

			fixedPacket := lowhttp.FixHTTPRequest([]byte(modifiedPacket))
			isHTTPS := inferGeneratedPacketHTTPS(loop, string(fixedPacket))
			reviewDecision := "auto_applied"

			if requireManualReview && loop.GetConfig().GetAllowUserInteraction() {
				question := fmt.Sprintf("请审核这次数据包修改是否应用。目标：%s\n修改原因：%s\n%s", modificationTarget, modificationReason, utils.ShrinkTextBlock(compareRequests(previousRequest, string(fixedPacket)), 600))
				suggestion := r.AskForClarification(getLoopTaskContext(loop), question, []string{"接受修改并继续使用新数据包", "拒绝修改并保留旧数据包"})
				if reviewSuggestionApproved(suggestion) {
					reviewDecision = "approved_by_user"
				} else {
					reviewDecision = "rejected_by_user"
					loop.Set("request_review_decision", buildReviewDecisionLabel(reviewDecision))
					feedback := "HTTP 数据包修改已生成人工审核请求，但用户未批准应用，当前仍保留旧数据包。\n\n"
					feedback += "=== 候选 Merge 变化 ===\n"
					feedback += compareRequests(previousRequest, string(fixedPacket))
					feedback += "\n审核结果："
					feedback += buildReviewDecisionLabel(reviewDecision)
					record := recordLoopHTTPFuzzMetaAction(
						loop,
						"modify_http_request",
						fmt.Sprintf("modification_target=%s; modification_reason=%s; require_manual_review=%v", modificationTarget, modificationReason, requireManualReview),
						utils.ShrinkTextBlock(feedback, 240),
					)
					persistLoopHTTPFuzzSessionContext(loop, "modify_http_request_rejected")
					operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + feedback)
					return
				}
			}

			fuzzReq, err := newLoopFuzzRequest(getLoopTaskContext(loop), r, fixedPacket, isHTTPS)
			if err != nil {
				operator.Fail(fmt.Errorf("failed to create modified FuzzHTTPRequest: %v", err))
				return
			}

			previousSummary := getCurrentRequestSummary(loop)
			setLoopCurrentRequestState(loop, fuzzReq, fixedPacket, isHTTPS)
			loop.Set("previous_request", previousRequest)
			loop.Set("previous_request_summary", previousSummary)
			loop.Set("request_change_summary", compareRequests(previousRequest, string(fixedPacket)))
			loop.Set("request_modification_reason", modificationReason)
			loop.Set("request_review_decision", buildReviewDecisionLabel(reviewDecision))
			loop.Set("bootstrap_source", "modify_http_request")

			feedback := buildRequestModificationFeedback([]byte(previousRequest), fixedPacket, isHTTPS, modificationReason, buildReviewDecisionLabel(reviewDecision))
			record := recordLoopHTTPFuzzMetaAction(
				loop,
				"modify_http_request",
				fmt.Sprintf("modification_target=%s; modification_reason=%s; require_manual_review=%v", modificationTarget, modificationReason, requireManualReview),
				utils.ShrinkTextBlock(compareRequests(previousRequest, string(fixedPacket)), 240),
			)
			persistLoopHTTPFuzzSessionContext(loop, "modify_http_request")
			log.Infof("modify_http_request action: target=%s, reason=%s, review=%v", modificationTarget, modificationReason, requireManualReview)
			r.AddToTimeline("modify_http_request", fmt.Sprintf("Modified current HTTP request: %s\n%s", modificationTarget, buildFuzzTimelineSummary(feedback)))
			operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + feedback)
		},
	)
}
