package loop_vuln_verify

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// recordFilterAction 记录过滤函数
func recordFilterAction(r aicommon.AIInvokeRuntime, state *VerifyState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"record_filter",
		"记录在数据流中发现的过滤/转义/验证函数，并评估其有效性。",
		[]aitool.ToolOption{
			aitool.WithStringParam("function",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("过滤函数名称，如 intval, htmlspecialchars, mysqli_real_escape_string 等")),
			aitool.WithStringParam("location",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("过滤函数所在位置，格式: 文件名:行号")),
			aitool.WithStringParam("filter_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("过滤类型: type_cast(类型转换), whitelist(白名单), blacklist(黑名单), regex(正则), escape(转义), custom(自定义)")),
			aitool.WithStringParam("effectiveness",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("有效性评估: effective(有效), ineffective(无效), uncertain(不确定)")),
			aitool.WithStringParam("note",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("补充说明，如为什么判断有效/无效、潜在绕过方式等")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			function := action.GetString("function")
			location := action.GetString("location")
			filterType := action.GetString("filter_type")
			effectiveness := action.GetString("effectiveness")
			note := action.GetString("note")

			// 验证 filter_type
			validFilterTypes := map[string]bool{
				"type_cast": true,
				"whitelist": true,
				"blacklist": true,
				"regex":     true,
				"escape":    true,
				"custom":    true,
			}
			if !validFilterTypes[filterType] {
				operator.Fail(fmt.Sprintf("无效的 filter_type: %s，有效值为: type_cast, whitelist, blacklist, regex, escape, custom", filterType))
				return
			}

			// 验证 effectiveness
			validEffectiveness := map[string]bool{
				"effective":   true,
				"ineffective": true,
				"uncertain":   true,
			}
			if !validEffectiveness[effectiveness] {
				operator.Fail(fmt.Sprintf("无效的 effectiveness: %s，有效值为: effective, ineffective, uncertain", effectiveness))
				return
			}

			// 创建过滤记录
			filter := FilterRecord{
				Function:      function,
				Location:      location,
				FilterType:    filterType,
				Effectiveness: effectiveness,
				Note:          note,
			}

			// 保存到状态
			state.AddFilter(filter)

			// 记录到时间线
			msg := fmt.Sprintf("发现过滤函数: %s @ %s (类型: %s, 有效性: %s)", function, location, filterType, effectiveness)
			if note != "" {
				msg += fmt.Sprintf("\n说明: %s", note)
			}
			r.AddToTimeline("filter", msg)

			log.Infof("[VulnVerify] Filter: %s @ %s (%s, %s)", function, location, filterType, effectiveness)

			// 根据有效性给出建议
			var feedback string
			switch effectiveness {
			case "effective":
				feedback = fmt.Sprintf("✓ 过滤函数已记录\n\n发现**有效**的过滤: %s\n这可能阻止漏洞利用。如果数据流中所有路径都经过有效过滤，可以使用 conclude 输出安全结论。", function)
			case "ineffective":
				feedback = fmt.Sprintf("✓ 过滤函数已记录\n\n发现**无效**的过滤: %s\n该过滤可能被绕过，漏洞可能仍然存在。继续分析并准备输出结论。", function)
			case "uncertain":
				feedback = fmt.Sprintf("✓ 过滤函数已记录\n\n发现**不确定**有效性的过滤: %s\n建议进一步分析该函数的实现，或使用 read_code 查看其源码。", function)
			}

			operator.Feedback(feedback)
		},
	)
}
