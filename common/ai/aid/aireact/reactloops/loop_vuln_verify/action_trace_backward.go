package loop_vuln_verify

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// traceBackwardAction 记录数据流追踪
func traceBackwardAction(r aicommon.AIInvokeRuntime, state *VerifyState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"trace_backward",
		"记录数据流追踪的一个节点。用于从 Sink 向上追踪参数的来源，逐步构建完整的数据流路径。",
		[]aitool.ToolOption{
			aitool.WithStringParam("variable",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("当前追踪的变量名")),
			aitool.WithStringParam("location",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("变量所在位置，格式: 文件名:行号")),
			aitool.WithStringParam("source",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("变量的数据来源，可以是另一个变量、函数返回值、用户输入等")),
			aitool.WithStringParam("note",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("补充说明，如是否经过处理、来源类型等")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			variable := action.GetString("variable")
			location := action.GetString("location")
			source := action.GetString("source")
			note := action.GetString("note")

			// 创建追踪记录
			record := TraceRecord{
				Variable: variable,
				Location: location,
				Source:   source,
				Note:     note,
			}

			// 保存到状态
			state.AddTraceRecord(record)

			// 记录到时间线
			msg := fmt.Sprintf("数据流追踪: %s @ %s <- %s", variable, location, source)
			if note != "" {
				msg += fmt.Sprintf(" (%s)", note)
			}
			r.AddToTimeline("trace", msg)

			log.Infof("[VulnVerify] Trace: %s @ %s <- %s", variable, location, source)

			// 计算当前追踪深度
			traceCount := len(state.TraceRecords)

			// 判断是否已追踪到 Source
			isUserInput := isLikelyUserInput(source)
			if isUserInput {
				operator.Feedback(fmt.Sprintf("✓ 追踪记录已添加 (第%d条)\n\n**发现可控的用户输入源: %s**\n\n数据流已追踪到用户可控的输入点，建议:\n1. 检查中间是否有过滤/转义\n2. 如已完成分析，使用 conclude 输出结论", traceCount, source))
			} else {
				operator.Feedback(fmt.Sprintf("✓ 追踪记录已添加 (第%d条)\n\n继续向上追踪 %s 的来源，或者:\n- 如果确认数据流完整，使用 conclude 输出结论\n- 如果发现过滤函数，使用 record_filter 记录", traceCount, source))
			}
		},
	)
}

// isLikelyUserInput 判断是否可能是用户输入
func isLikelyUserInput(source string) bool {
	userInputPatterns := []string{
		"$_GET", "$_POST", "$_REQUEST", "$_COOKIE", "$_FILES", "$_SERVER",
		"request.getParameter", "request.getAttribute", "request.getHeader",
		"request.getCookies", "request.getInputStream",
		"req.body", "req.query", "req.params", "req.cookies", "req.headers",
		"request.form", "request.args", "request.json", "request.data",
		"@RequestParam", "@PathVariable", "@RequestBody", "@RequestHeader",
		"user input", "user_input", "userinput",
	}

	sourceLower := strings.ToLower(source)
	for _, pattern := range userInputPatterns {
		if strings.Contains(sourceLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
