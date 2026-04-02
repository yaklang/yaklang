package loop_code_security_audit

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase3_verify_instruction.txt
var phase3VerifyInstruction string

//go:embed prompts/phase3_output_example.txt
var phase3OutputExample string

// phase3ReactiveDataTpl 是每轮注入的动态验证进度状态
const phase3ReactiveDataTpl = `## 当前验证进度
<|VERIFY_STATUS_{{ .Nonce }}|>
**总计 Findings**: {{ .TotalFindings }} 个
**已验证**: {{ .VerifiedCount }} 个（confirmed: {{ .ConfirmedCount }}，uncertain: {{ .UncertainCount }}，safe: {{ .SafeCount }}）
**待验证**: {{ .RemainingCount }} 个
**当前迭代**: {{ .IterationCount }}

**技术栈**: {{ .TechStack }}
**入口点**: {{ .EntryPoints }}
{{ if .AuthMechanism }}**认证机制**: {{ .AuthMechanism }}{{ end }}

> [路径规则] 所有工具路径必须使用用户指定的项目绝对路径，且注意各工具参数名不同：
> - read_file 使用 file 参数（不是 path）
> - grep 使用 path 参数
> Finding 中的 file 字段通常是相对路径，调用工具前必须手动拼接用户指定的项目根目录。
{{ if .ReconOutline }}
**项目背景报告章节大纲**（报告已持久化，包含以下章节）:
{{ .ReconOutline }}
> 验证时如需路由映射、中间件链、数据访问模式等信息，优先调用 read_recon_notes 读取完整报告，比重新 grep 代码更高效。
{{ else if .ReconFileHint }}
**项目背景报告**: {{ .ReconFileHint }}（包含路由列表、数据访问模式、认证机制等背景信息）
> 验证时如需了解路由映射、认证机制等背景，优先调用 read_recon_notes，而非重新扫描代码。
{{ end }}

{{ if .PendingFindings }}
### 待验证的 Findings（按 ID 顺序处理）
{{ .PendingFindings }}
{{ end }}

{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|VERIFY_STATUS_END_{{ .Nonce }}|>
{{ if .ShouldForceConclusion }}
[警告] 迭代次数已超过 80%。立即对当前正在验证的 Finding 调用 conclude_finding（可用 uncertain），然后依次完成剩余 Finding，最后调用 complete_verify。
{{ end }}
请从第一个待验证的 Finding 开始，依次调用 read_file/grep（不超过5次/Finding） → trace_data_flow → note_filter（如有） → conclude_finding。全部完成后调用 complete_verify。`

// buildPhase3VerifyLoop 构建 Phase 3 验证 Loop
// 目标：遍历所有 findings，逐个读取代码、追踪数据流、确认/排除漏洞
func buildPhase3VerifyLoop(r aicommon.AIInvokeRuntime, state *AuditState, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	maxIter := math.MaxInt32

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		// 关闭通用 tool call，所有文件读取通过 read_code action 进行
		// （read_code 内部使用 Yak read_file 工具或降级到内联实现）
		reactloops.WithAllowToolCall(false), // file ops handled by explicit read_file / grep loop actions
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(true),
		reactloops.WithSameActionTypeSpinThreshold(3),
		reactloops.WithSameLogicSpinThreshold(2),
		reactloops.WithMaxConsecutiveSpinWarnings(2),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			return utils.RenderTemplate(phase3VerifyInstruction, map[string]any{
				"Nonce":       nonce,
				"ReconFile":   state.GetReconFilePath(),
				"TechStack":   state.TechStack,
				"EntryPoints": state.EntryPoints,
			})
		}),
		reactloops.WithReflectionOutputExample(phase3OutputExample),

		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			allFindings := state.GetFindings()
			verifiedVulns := state.GetVerifiedVulns()
			stats := state.GetStats()
			iterCount := loop.GetCurrentIterationIndex()

			// 计算已验证 finding 的 ID 集合
			verifiedIDs := make(map[string]bool, len(verifiedVulns))
			for _, vf := range verifiedVulns {
				if vf.Finding != nil {
					verifiedIDs[vf.Finding.ID] = true
				}
			}

			// 构建待验证 finding 摘要
			var pendingBuf strings.Builder
			for _, f := range allFindings {
				if !verifiedIDs[f.ID] {
					pendingBuf.WriteString(fmt.Sprintf("- %s [%s] %s: %s (%s:%d)\n",
						f.ID, f.Severity, f.Category, f.Title, f.File, f.Line))
				}
			}

			reconFileHint := ""
			if p := state.GetReconFilePath(); p != "" {
				reconFileHint = p + "（调用 read_recon_notes 读取）"
			}
			return utils.RenderTemplate(phase3ReactiveDataTpl, map[string]any{
				"Nonce":                 nonce,
				"TotalFindings":         len(allFindings),
				"VerifiedCount":         len(verifiedVulns),
				"ConfirmedCount":        stats.ConfirmedCount,
				"UncertainCount":        stats.UncertainCount,
				"SafeCount":             stats.SafeCount,
				"RemainingCount":        len(allFindings) - len(verifiedVulns),
				"PendingFindings":       pendingBuf.String(),
				"FeedbackMessages":      feedbacker.String(),
				"IterationCount":        iterCount,
				"ReconOutline":          state.GetReconOutline(),
				"ReconFileHint":         reconFileHint,
				"TechStack":             state.TechStack,
				"EntryPoints":           state.EntryPoints,
				"AuthMechanism":         state.AuthMechanism,
				"ShouldForceConclusion": false,
			})
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			findings := state.GetFindings()
			log.Infof("[CodeAudit/Phase3] Verify loop started, %d findings to verify", len(findings))

			if len(findings) == 0 {
				r.AddToTimeline("[VERIFY_INIT]", "没有 finding 需要验证，跳过验证阶段。")
				op.Done()
				return
			}

			// 将全量 findings JSON 写入 timeline，AI 从这里获得完整信息
			findingsJSON, _ := json.MarshalIndent(findings, "", "  ")
			r.AddToTimeline("[VERIFY_INIT]", fmt.Sprintf(
				"Phase 3 验证开始。共 %d 个 finding 需要依次验证。\n\n完整 Finding 列表：\n```json\n%s\n```",
				len(findings), string(findingsJSON)))
			op.Continue()
		}),

		// 文件系统工具（直接使用 Yak 脚本工具 read_file / grep，不再封装为 read_code）
		buildFSAction(r, "read_file"),
		buildFSAction(r, "grep"),

		// trace_data_flow: 记录数据流追踪节点
		reactloops.WithRegisterLoopAction(
			"trace_data_flow",
			"记录数据流追踪的一个节点（从 Sink 向 Source 逐步追踪，每追一步调用一次）。",
			[]aitool.ToolOption{
				aitool.WithIntegerParam("step",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("当前是第几步追踪（从 1 开始）")),
				aitool.WithStringParam("variable",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("当前追踪的变量名")),
				aitool.WithStringParam("trace_location",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("变量位置：文件名:行号，如 handler/user.go:55")),
				aitool.WithStringParam("source_type",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("来源类型: http_param / request_body / cookie / function_param / config / hardcoded")),
				aitool.WithStringParam("trace_note",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("说明：是否经过处理、是否可控等")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				step := int(action.GetInt("step"))
				variable := action.GetString("variable")
				location := action.GetString("trace_location")
				sourceType := action.GetString("source_type")
				note := action.GetString("trace_note")
				msg := fmt.Sprintf("Step %d: %s @ %s [source: %s]", step, variable, location, sourceType)
				if note != "" {
					msg += " → " + note
				}
				r.AddToTimeline("trace", msg)
				op.Feedback(fmt.Sprintf("✓ 数据流节点已记录：%s", msg))
			},
		),

		// note_filter: 记录发现的过滤函数
		reactloops.WithRegisterLoopAction(
			"note_filter",
			"记录数据流中发现的过滤/转义/校验函数及其有效性评估。",
			[]aitool.ToolOption{
				aitool.WithStringParam("filter_location",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("过滤函数位置：文件名:行号")),
				aitool.WithStringParam("filter_type",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("过滤类型: type_cast / whitelist / parameterized / escape / blacklist / regex / custom")),
				aitool.WithStringParam("effectiveness",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("有效性: effective(有效) / ineffective(无效) / uncertain(不确定)")),
				aitool.WithStringParam("filter_note",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("说明：为何有效/无效，是否存在绕过方法")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				location := action.GetString("filter_location")
				filterType := action.GetString("filter_type")
				effectiveness := action.GetString("effectiveness")
				note := action.GetString("filter_note")
				msg := fmt.Sprintf("过滤 [%s] @ %s [%s]", filterType, location, effectiveness)
				if note != "" {
					msg += " → " + note
				}
				r.AddToTimeline("filter", msg)
				op.Feedback(fmt.Sprintf("✓ 过滤函数已记录：%s", msg))
			},
		),

		// conclude_finding: 对当前 finding 输出验证结论
		reactloops.WithRegisterLoopAction(
			"conclude_finding",
			"对当前 finding 输出最终验证结论，结果写入 verified_vulns 列表。每个 finding 有且只有一次调用。",
			[]aitool.ToolOption{
				aitool.WithStringParam("finding_id",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("被验证的 finding ID，如 VULN-001")),
				aitool.WithStringParam("status",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("验证结论（必须为以下之一）: confirmed / safe / uncertain")),
				aitool.WithIntegerParam("confidence",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("验证后置信度 1-10（confirmed ≥ 7，uncertain 5-6，safe ≤ 4）")),
				aitool.WithStringParam("reason",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("详细验证理由：数据流完整性、过滤有效性、可控性分析")),
				aitool.WithStringParam("data_flow",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("完整验证后的数据流路径（比扫描阶段更精确）")),
				aitool.WithStringParam("exploit",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("confirmed 时：具体利用方式和 payload")),
				aitool.WithStringParam("fix",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("修复建议（代码示例）")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				findingID := action.GetString("finding_id")
				status := VerifyStatus(action.GetString("status"))
				confidence := int(action.GetInt("confidence"))
				reason := action.GetString("reason")
				dataFlow := action.GetString("data_flow")
				exploit := action.GetString("exploit")
				fix := action.GetString("fix")

				validStatuses := map[VerifyStatus]bool{
					VerifyConfirmed: true,
					VerifySafe:      true,
					VerifyUncertain: true,
				}
				if !validStatuses[status] {
					// 不用 Fail 终止 loop，给 AI 反馈让其纠正
					op.Feedback(fmt.Sprintf("无效 status: %q，有效值: confirmed / safe / uncertain，请重新提交。", status))
					return
				}
				if confidence < 1 {
					confidence = 1
				}
				if confidence > 10 {
					confidence = 10
				}

				f := state.GetFindingByID(findingID)
				if f == nil {
					// 不用 Fail 终止 loop，给 AI 反馈让其纠正 finding_id
					op.Feedback(fmt.Sprintf("finding_id %q 不存在，请从待验证列表中选择正确的 ID 重试。", findingID))
					return
				}
				if dataFlow == "" {
					dataFlow = f.DataFlow
				}
				if fix == "" {
					fix = f.Recommendation
				}

				vf := &VerifiedFinding{
					Finding:    f,
					Status:     status,
					Confidence: confidence,
					Reason:     reason,
					DataFlow:   dataFlow,
					Exploit:    exploit,
					Fix:        fix,
				}
				state.AddVerifiedFinding(vf)

				loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_verify_finding", map[string]any{
					"finding_id": findingID,
					"status":     string(status),
					"confidence": confidence,
					"reason":     reason,
				})

				r.AddToTimeline("[CONCLUDE_FINDING]",
					fmt.Sprintf("Finding %s 验证结论: %s (置信度: %d/10)\n%s", findingID, status, confidence, reason))

				log.Infof("[CodeAudit/Phase3] Finding %s: %s (confidence %d)", findingID, status, confidence)

				allFindings := state.GetFindings()
				verifiedCount := len(state.GetVerifiedVulns())
				remaining := len(allFindings) - verifiedCount
				if remaining > 0 {
					op.Feedback(fmt.Sprintf("Finding %s 验证完成: %s（置信度 %d/10）。\n还有 %d 个 finding 待验证，请继续。", findingID, status, confidence, remaining))
				} else {
					op.Feedback(fmt.Sprintf("Finding %s 验证完成: %s（置信度 %d/10）。\n所有 %d 个 finding 已验证完毕，请调用 complete_verify。", findingID, status, confidence, len(allFindings)))
				}
			},
		),

		reactloops.WithRegisterLoopAction(
			"read_recon_notes",
			"读取项目背景报告（包含路由列表、中间件链、数据库访问模式、认证机制等）。当需要了解路由映射或权限控制时优先调用，比重新 grep 代码更高效。",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				content, err := state.GetReconFileContent()
				if err != nil {
					op.Feedback(fmt.Sprintf("无法读取项目背景报告: %v\n请直接使用 read_file/grep 查找所需信息。", err))
					return
				}
				r.AddToTimeline("read_recon_notes", fmt.Sprintf("[Phase3] 读取项目背景报告 (%d 字节)", len(content)))
				op.Feedback("=== 项目背景报告 ===\n\n" + content)
			},
		),

		// complete_verify: 验证阶段完成
		reactloops.WithRegisterLoopAction(
			"complete_verify",
			"所有 findings 验证完成后调用此 action，进入报告生成阶段。",
			[]aitool.ToolOption{
				aitool.WithStringParam("summary",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("验证总结：confirmed/uncertain/safe 各多少个，主要发现")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				verified := state.GetVerifiedVulns()
				confirmed := state.GetConfirmedVulns()
				stats := state.GetStats()
				summary := action.GetString("summary")

				state.SetPhase(AuditPhaseReport)
				r.AddToTimeline("[VERIFY_COMPLETE]", fmt.Sprintf(
					"Phase 3 验证完成。%s\n确认: %d，uncertain: %d，safe: %d（HIGH:%d MEDIUM:%d LOW:%d）",
					summary, len(confirmed), stats.UncertainCount, stats.SafeCount,
					stats.HighCount, stats.MediumCount, stats.LowCount))

				log.Infof("[CodeAudit/Phase3] Verify complete. total=%d confirmed=%d uncertain=%d safe=%d",
					len(verified), len(confirmed), stats.UncertainCount, stats.SafeCount)
				op.Feedback("验证完成，进入报告生成阶段。")
				op.Exit()
			},
		),
	}
	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase3_verify", r, preset...)
}
