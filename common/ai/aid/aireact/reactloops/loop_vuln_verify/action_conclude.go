package loop_vuln_verify

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// concludeAction 输出验证结论
func concludeAction(r aicommon.AIInvokeRuntime, state *VerifyState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"conclude",
		"输出最终的漏洞验证结论。这是验证流程的最后一步。",
		[]aitool.ToolOption{
			aitool.WithStringParam("result",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("验证结果: confirmed(确认漏洞), safe(安全), uncertain(需人工确认)")),
			aitool.WithStringParam("confidence",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("置信度: high(高), medium(中), low(低)")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("详细的验证理由，包括数据流分析、过滤分析等")),
			aitool.WithStringParam("exploit_condition",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("如果是确认漏洞，描述利用条件和方法")),
			aitool.WithStringParam("fix_suggestion",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("修复建议")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			// 检查是否设置了漏洞上下文
			if state.GetVulnContext() == nil {
				return fmt.Errorf("请先使用 set_vuln_context 设置漏洞上下文")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			result := action.GetString("result")
			confidence := action.GetString("confidence")
			reason := action.GetString("reason")
			exploitCondition := action.GetString("exploit_condition")
			fixSuggestion := action.GetString("fix_suggestion")

			// 验证 result
			validResults := map[string]bool{
				"confirmed": true,
				"safe":      true,
				"uncertain": true,
			}
			if !validResults[result] {
				operator.Fail(fmt.Sprintf("无效的 result: %s，有效值为: confirmed, safe, uncertain", result))
				return
			}

			// 验证 confidence
			validConfidence := map[string]bool{
				"high":   true,
				"medium": true,
				"low":    true,
			}
			if !validConfidence[confidence] {
				operator.Fail(fmt.Sprintf("无效的 confidence: %s，有效值为: high, medium, low", confidence))
				return
			}

			// 创建结论
			conclusion := &Conclusion{
				Result:           result,
				Confidence:       confidence,
				Reason:           reason,
				ExploitCondition: exploitCondition,
				FixSuggestion:    fixSuggestion,
			}

			// 保存到状态
			state.SetConclusion(conclusion)

			// 获取漏洞上下文
			ctx := state.GetVulnContext()

			// 构建完整的结论报告
			var report string
			switch result {
			case "confirmed":
				report = fmt.Sprintf("## 🔴 漏洞确认\n\n**位置**: %s:%d\n**类型**: %s\n**Sink**: %s\n**置信度**: %s\n\n### 验证理由\n%s",
					ctx.FilePath, ctx.Line, ctx.VulnType, ctx.SinkFunction, confidence, reason)
				if exploitCondition != "" {
					report += fmt.Sprintf("\n\n### 利用条件\n%s", exploitCondition)
				}
				if fixSuggestion != "" {
					report += fmt.Sprintf("\n\n### 修复建议\n%s", fixSuggestion)
				}
			case "safe":
				report = fmt.Sprintf("## 🟢 安全确认\n\n**位置**: %s:%d\n**类型**: %s\n**置信度**: %s\n\n### 验证理由\n%s",
					ctx.FilePath, ctx.Line, ctx.VulnType, confidence, reason)
			case "uncertain":
				report = fmt.Sprintf("## 🟡 需人工确认\n\n**位置**: %s:%d\n**类型**: %s\n**置信度**: %s\n\n### 不确定原因\n%s",
					ctx.FilePath, ctx.Line, ctx.VulnType, confidence, reason)
				if exploitCondition != "" {
					report += fmt.Sprintf("\n\n### 可能的利用条件\n%s", exploitCondition)
				}
			}

			// 添加数据流追踪摘要
			if len(state.TraceRecords) > 0 {
				report += "\n\n### 数据流追踪\n"
				for i, record := range state.TraceRecords {
					report += fmt.Sprintf("%d. %s @ %s ← %s\n", i+1, record.Variable, record.Location, record.Source)
				}
			}

			// 添加过滤函数摘要
			if len(state.Filters) > 0 {
				report += "\n\n### 发现的过滤函数\n"
				for _, filter := range state.Filters {
					report += fmt.Sprintf("- %s @ %s (%s, %s)\n", filter.Function, filter.Location, filter.FilterType, filter.Effectiveness)
				}
			}

			// 记录到时间线
			r.AddToTimeline("conclusion", report)

			log.Infof("[VulnVerify] Conclusion: %s (%s confidence)", result, confidence)

			// 发送结构化事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "vuln_verify_conclusion", map[string]any{
				"vuln_context": ctx,
				"result":       result,
				"confidence":   confidence,
				"reason":       reason,
				"conclusion":   conclusion,
				"state":        state.ToJSON(),
			})

			// 完成验证
			operator.Feedback(report + "\n\n---\n验证完成。")
			// 默认允许退出，无需调用 AllowNextLoopExit
		},
	)
}
