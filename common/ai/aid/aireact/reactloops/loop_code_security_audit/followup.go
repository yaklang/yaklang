package loop_code_security_audit

import (
	"bytes"
	_ "embed"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/followup_instruction.txt
var followupInstruction string

const followupOutputExample = `
* 当用户追问某段代码是否安全时：
  {"@action": "require_tool", "tool": "read_file", "params": {"file": "/abs/path/to/file.go"}, "human_readable_thought": "读取用户关注的文件以结合审计结论分析"}
* 当信息已足够回答时：
  {"@action": "directly_answer", "answer_payload": "...", "human_readable_thought": "基于审计报告与选区代码给出结论"}
`

const followupReactiveDataTpl = `## 审计后追问模式
<|AUDIT_FOLLOWUP_{{ .Nonce }}|>

**用户问题**: {{ .UserQuery }}

**项目**: {{ .ProjectName }}（{{ .ProjectPath }}）
**技术栈**: {{ .TechStack }}

| 指标 | 数量 |
|------|------|
| 已确认漏洞 | {{ .ConfirmedCount }} |
| 需人工确认 | {{ .UncertainCount }} |
| 已排除 | {{ .SafeCount }} |
| 高危/中危/低危 | {{ .HighCount }} / {{ .MediumCount }} / {{ .LowCount }} |

**审计数据文件**（可用 read_file 或 read_audit_report 读取）:
{{ if .ReportPath }}- 报告: {{ .ReportPath }}
{{ end }}{{ if .VerifiedVulnsPath }}- 验证结果: {{ .VerifiedVulnsPath }}
{{ end }}{{ if .FindingsPath }}- 原始 findings: {{ .FindingsPath }}
{{ end }}{{ if .ReconPath }}- 项目背景: {{ .ReconPath }}
{{ end }}
> 用户可能在消息中附带代码文件/选区，请结合审计结论分析。工具路径须使用项目绝对路径。

{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|AUDIT_FOLLOWUP_END_{{ .Nonce }}|>
请基于审计结果与用户问题作答；需要更多代码细节时使用 read_file/grep。`

// buildFollowUpLoop runs interactive Q&A after the main audit pipeline has completed.
// The frontend may keep focus mode locked to code_security_audit; this sub-loop handles
// follow-up turns without re-entering Phase 1–4.
func buildFollowUpLoop(r aicommon.AIInvokeRuntime, state *AuditState, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
		reactloops.WithEnableSelfReflection(true),
		reactloops.WithSameActionTypeSpinThreshold(5),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),
		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			return followupInstruction, nil
		}),
		reactloops.WithReflectionOutputExample(followupOutputExample),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			task := loop.GetCurrentTask()
			userQuery := ""
			if task != nil {
				userQuery = strings.TrimSpace(task.GetUserInput())
			}
			stats := state.GetStats()
			return utils.RenderTemplate(followupReactiveDataTpl, map[string]any{
				"Nonce":             nonce,
				"UserQuery":         userQuery,
				"ProjectPath":       state.ProjectPath,
				"ProjectName":       state.ProjectName,
				"TechStack":         state.TechStack,
				"ConfirmedCount":    stats.ConfirmedCount,
				"UncertainCount":    stats.UncertainCount,
				"SafeCount":         stats.SafeCount,
				"HighCount":         stats.HighCount,
				"MediumCount":       stats.MediumCount,
				"LowCount":          stats.LowCount,
				"ReportPath":        state.GetFinalReportPath(),
				"VerifiedVulnsPath": state.GetVerifiedVulnsFilePath(),
				"FindingsPath":      state.GetFindingsFilePath(),
				"ReconPath":         state.GetReconFilePath(),
				"FeedbackMessages":  strings.TrimSpace(feedbacker.String()),
			})
		}),
		buildReadAuditReportAction(state),
	}

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_followup", r, preset...)
}

func buildReadAuditReportAction(state *AuditState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_audit_report",
		"读取已生成的安全审计 Markdown 报告全文。追问时优先调用以获取漏洞摘要与修复建议。",
		nil,
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			path := state.GetFinalReportPath()
			if path == "" {
				op.Feedback("审计报告尚未生成或路径未知，请改用 read_file 读取 audit 目录下的 security_audit_report.md。")
				return
			}
			content, err := os.ReadFile(path)
			if err != nil {
				op.Feedback(fmt.Sprintf("读取审计报告失败 %s: %v", path, err))
				return
			}
			op.Feedback(utils.ShrinkTextBlock(string(content), 12000))
		},
	)
}
