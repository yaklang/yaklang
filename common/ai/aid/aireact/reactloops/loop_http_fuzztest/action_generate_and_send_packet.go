package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const generatedPacketContentField = "generated_packet_content"

var generateAndSendPacketAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"generate_and_send_packet",
		"生成完整原始 HTTP 数据包并直接发送测试。适用于需要 AI 基于漏洞目标构造整包请求的场景。",
		[]aitool.ToolOption{
			aitool.WithStringParam("packet_type", aitool.WithParam_Description("数据包类型：mutation 表示基于当前请求变异，synthetic 表示重新生成完整原始包。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("target_purpose", aitool.WithParam_Description("请用中文说明这次整包测试的漏洞目标，例如‘验证登录接口 SQL 注入’。"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("generation_reason", aitool.WithParam_Description("请用中文说明为何要发送这个完整数据包、怀疑的漏洞类型、以及必须遵守的安全边界。"), aitool.WithParam_Required(true)),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "packet_type", AINodeId: "thought"},
			{FieldName: "target_purpose", AINodeId: "thought"},
			{FieldName: "generation_reason", AINodeId: "thought"},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			action.WaitStream(l.GetCurrentTask().GetContext())

			packetType := strings.TrimSpace(action.GetString("packet_type"))
			if packetType != "mutation" && packetType != "synthetic" {
				return fmt.Errorf("packet_type must be one of: mutation, synthetic")
			}
			if strings.TrimSpace(action.GetString("target_purpose")) == "" {
				return fmt.Errorf("target_purpose is required")
			}
			if strings.TrimSpace(action.GetString("generation_reason")) == "" {
				return fmt.Errorf("generation_reason is required")
			}
			packet := strings.TrimSpace(action.GetString(generatedPacketContentField))
			if packet == "" {
				packet = strings.TrimSpace(l.Get(generatedPacketContentField))
			}
			if packet == "" {
				return fmt.Errorf("GEN_PACKET AITAG is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			packetType := action.GetString("packet_type")
			targetPurpose := action.GetString("target_purpose")
			generationReason := action.GetString("generation_reason")
			rawPacket := strings.TrimSpace(action.GetString(generatedPacketContentField))
			if rawPacket == "" {
				rawPacket = strings.TrimSpace(loop.Get(generatedPacketContentField))
			}
			if rawPacket == "" {
				operator.Fail(fmt.Errorf("generated packet content is empty"))
				return
			}

			isHTTPS := inferGeneratedPacketHTTPS(loop, rawPacket)
			fuzzReq, err := newLoopFuzzRequest(getLoopTaskContext(loop), r, []byte(rawPacket), isHTTPS)
			if err != nil {
				operator.Fail(fmt.Errorf("failed to create generated packet request: %v", err))
				return
			}

			storeLoopFuzzRequestState(loop, fuzzReq, []byte(rawPacket), isHTTPS)
			loop.Set("bootstrap_source", "generated_packet_action")
			emitLoopHTTPFuzzEditablePacket(loop, operator.GetTask(), rawPacket)
			persistLoopHTTPFuzzSessionContext(loop, "generated_packet_action")

			log.Infof("generate_and_send_packet action: packet_type=%s, target=%s, reason=%s", packetType, targetPurpose, generationReason)

			paramSummary := fmt.Sprintf("packet_type=%s; target_purpose=%s; generation_reason=%s", packetType, targetPurpose, generationReason)
			diffResult, verifyResult, err := executeFuzzAndCompare(loop, fuzzReq.Repeat(1), "generate_and_send_packet", paramSummary)
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline(
				"generate_and_send_packet",
				fmt.Sprintf("Sent generated packet (%s): %s\n%s", packetType, targetPurpose, buildFuzzTimelineSummary(diffResult)),
			)
			applyFuzzVerificationOutcome(loop, operator, diffResult, verifyResult)
		},
	)
}

func inferGeneratedPacketHTTPS(loop *reactloops.ReActLoop, rawPacket string) bool {
	if loop != nil {
		if strings.EqualFold(loop.Get("is_https"), "true") {
			return true
		}
	}
	packetLower := strings.ToLower(rawPacket)
	if strings.Contains(packetLower, "https://") {
		return true
	}
	if urlObj, err := lowhttp.ExtractURLFromHTTPRequestRaw([]byte(rawPacket), true); err == nil && urlObj != nil {
		return strings.EqualFold(urlObj.Scheme, "https")
	}
	return false
}
