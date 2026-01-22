package loop_vuln_verify

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// setVulnContextAction 设置漏洞上下文
func setVulnContextAction(r aicommon.AIInvokeRuntime, state *VerifyState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_vuln_context",
		"设置待验证的漏洞信息，包括文件路径、行号、sink函数、漏洞类型等。这是验证流程的第一步。",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("漏洞所在的文件路径")),
			aitool.WithIntegerParam("line",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("漏洞所在的行号")),
			aitool.WithStringParam("sink_function",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("危险函数名称，如 mysqli_query, system, eval 等")),
			aitool.WithStringParam("vuln_type",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("漏洞类型，如 SQL Injection, Command Injection, XSS 等")),
			aitool.WithStringParam("description",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("漏洞的初步描述")),
			aitool.WithStringParam("suspected_source",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("疑似的数据源，如 $_GET['id'], request.getParameter 等")),
		},
		nil, // 无预检查
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file_path")
			line := action.GetInt("line")
			sinkFunction := action.GetString("sink_function")
			vulnType := action.GetString("vuln_type")
			description := action.GetString("description")
			suspectedSource := action.GetString("suspected_source")

			// 创建漏洞上下文
			ctx := &VulnContext{
				FilePath:        filePath,
				Line:            int(line),
				SinkFunction:    sinkFunction,
				VulnType:        vulnType,
				Description:     description,
				SuspectedSource: suspectedSource,
			}

			// 保存到状态
			state.SetVulnContext(ctx)

			// 记录到时间线
			msg := fmt.Sprintf("设置漏洞上下文:\n- 文件: %s:%d\n- Sink: %s\n- 类型: %s\n- 描述: %s",
				filePath, line, sinkFunction, vulnType, description)
			if suspectedSource != "" {
				msg += fmt.Sprintf("\n- 疑似Source: %s", suspectedSource)
			}
			r.AddToTimeline("set_context", msg)

			log.Infof("[VulnVerify] Context set: %s:%d (%s)", filePath, line, vulnType)

			operator.Feedback(fmt.Sprintf("漏洞上下文已设置。\n\n下一步建议:\n1. 使用 read_code 读取 %s 的相关代码\n2. 从 Sink 点开始向上追踪数据流", filePath))
		},
	)
}
