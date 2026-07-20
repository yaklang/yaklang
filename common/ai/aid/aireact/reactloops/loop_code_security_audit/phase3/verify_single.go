package phase3

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
		"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// phase3SingleFindingReactiveDataTpl is injected each iteration for one forked finding verify sub-agent.
const phase3SingleFindingReactiveDataTpl = `## 当前验证任务（子 Agent {{ .FindingIndex }}/{{ .FindingTotal }}）
<|VERIFY_STATUS_{{ .Nonce }}|>
**本任务 Finding**: {{ .FindingID }}
{{ if .FindingSummary }}
{{ .FindingSummary }}
{{ end }}

**全局进度**: 已验证 {{ .GlobalVerifiedCount }}/{{ .GlobalTotalFindings }} 个 finding
**当前迭代**: {{ .IterationCount }}

**技术栈**: {{ .TechStack }}
**入口点**: {{ .EntryPoints }}
{{ if .AuthMechanism }}**认证机制**: {{ .AuthMechanism }}{{ end }}

> [路径规则] 所有工具路径必须使用用户指定的项目绝对路径，且注意各工具参数名不同：
> - read_file 使用 file 参数（不是 path）
> - grep 使用 path 参数
> Finding 中的 file 字段通常是相对路径，调用工具前必须手动拼接用户指定的项目根目录。
{{ if .ReconOutline }}
**项目背景报告章节大纲**:
{{ .ReconOutline }}
> 验证时如需路由映射、中间件链、数据访问模式等信息，优先调用 read_recon_notes。
{{ else if .ReconFileHint }}
**项目背景报告**: {{ .ReconFileHint }}
{{ end }}

{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|VERIFY_STATUS_END_{{ .Nonce }}|>
[终止规则] 对本 finding 完成 read_file/grep（≤5次）→ trace_data_flow → note_filter（如有）→ **conclude_finding** 后子任务结束。`

func buildSingleFindingVerifyLoop(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	finding *model.Finding,
	findingIndex, findingTotal int,
) (*reactloops.ReActLoop, error) {
	if finding == nil || finding.ID == "" {
		return nil, utils.Error("finding is nil or missing ID")
	}

	maxIter := math.MaxInt32
	verify := newVerifyState([]*model.Finding{finding})
	concluded := false

	findingSummary := fmt.Sprintf("- [%s] %s: %s (%s:%d)",
		finding.Severity, finding.Category, finding.Title, finding.File, finding.Line)

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(true),
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
			reconFileHint := ""
			if p := state.GetReconFilePath(); p != "" {
				reconFileHint = p + "（调用 read_recon_notes 读取）"
			}
			return utils.RenderTemplate(phase3SingleFindingReactiveDataTpl, map[string]any{
				"Nonce":               nonce,
				"FindingID":           finding.ID,
				"FindingSummary":      findingSummary,
				"FindingIndex":        findingIndex,
				"FindingTotal":        findingTotal,
				"GlobalTotalFindings": len(state.GetFindings()),
				"GlobalVerifiedCount": len(state.GetVerifiedVulns()),
				"FeedbackMessages":    feedbacker.String(),
				"IterationCount":      loop.GetCurrentIterationIndex(),
				"ReconOutline":        state.GetReconOutline(),
				"ReconFileHint":       reconFileHint,
				"TechStack":           state.TechStack,
				"EntryPoints":         state.EntryPoints,
				"AuthMechanism":       state.AuthMechanism,
			})
		}),

		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
			if !isDone || concluded {
				return
			}
			FinalizeOnLoopEnd(r, state, verify, concluded, reason)
			if verify.IsConcluded(finding.ID) {
				concluded = true
			}
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			log.Infof("[CodeAudit/Phase3/%s] Verify sub-agent started", finding.ID)
			findingJSON, _ := json.MarshalIndent(finding, "", "  ")
			r.AddToTimeline("[VERIFY_FINDING_START]", fmt.Sprintf(
				"开始验证 %s。\n\n```json\n%s\n```", finding.ID, string(findingJSON)))
			op.Continue()
		}),

		registerPhase3TraceDataFlowAction(r),
		registerPhase3NoteFilterAction(r),
		registerPhase3ConcludeFindingAction(r, state, verify, &concluded),
		registerPhase3ReadReconNotesAction(r, state),
	}


	loopName := fmt.Sprintf("code_audit_verify_%s", finding.ID)
	return reactloops.NewReActLoop(loopName, r, preset...)
}

func registerPhase3TraceDataFlowAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
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
			log.Infof("[CodeAudit/Phase3] trace: %s", msg)
			r.AddToTimeline("trace", msg)
			op.Feedback(fmt.Sprintf("数据流节点已记录：%s", msg))
		},
	)
}

func registerPhase3NoteFilterAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
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
			log.Infof("[CodeAudit/Phase3] filter: %s", msg)
			r.AddToTimeline("filter", msg)
			op.Feedback(fmt.Sprintf("过滤函数已记录：%s", msg))
		},
	)
}

func registerPhase3ConcludeFindingAction(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	verify *VerifyState,
	concluded *bool,
) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
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
			status := model.VerifyStatus(action.GetString("status"))
			confidence := int(action.GetInt("confidence"))
			reason := action.GetString("reason")
			dataFlow := action.GetString("data_flow")
			exploit := action.GetString("exploit")
			fix := action.GetString("fix")

			validStatuses := map[model.VerifyStatus]bool{
				model.VerifyConfirmed: true,
				model.VerifySafe:      true,
				model.VerifyUncertain: true,
			}
			if !validStatuses[status] {
				op.Feedback(fmt.Sprintf("无效 status: %q，有效值: confirmed / safe / uncertain，请重新提交。", status))
				return
			}
			if confidence < 1 {
				confidence = 1
			}
			if confidence > 10 {
				confidence = 10
			}

			if ok, msg := verify.CanConclude(findingID); !ok {
				op.Feedback(msg)
				return
			}

			f := state.GetFindingByID(findingID)
			if f == nil {
				op.Feedback(fmt.Sprintf("finding_id %q 不存在，请从待验证列表中选择正确的 ID 重试。", findingID))
				return
			}
			if dataFlow == "" {
				dataFlow = f.DataFlow
			}
			if fix == "" {
				fix = f.Recommendation
			}

			vf := &model.VerifiedFinding{
				Finding:    f,
				Status:     status,
				Confidence: confidence,
				Reason:     reason,
				DataFlow:   dataFlow,
				Exploit:    exploit,
				Fix:        fix,
			}
			state.UpsertVerifiedFinding(vf)
			verify.MarkConcluded(findingID)
			*concluded = true

			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_verify_finding", map[string]any{
				"finding_id": findingID,
				"status":     string(status),
				"confidence": confidence,
				"reason":     reason,
			})

			r.AddToTimeline("[CONCLUDE_FINDING]",
				fmt.Sprintf("Finding %s 验证结论: %s (置信度: %d/10)\n%s", findingID, status, confidence, reason))
			log.Infof("[CodeAudit/Phase3] Finding %s: %s (confidence %d)", findingID, status, confidence)

			reasonPreview := utils.ShrinkTextBlock(reason, 300)
			op.Feedback(fmt.Sprintf("Finding %s 验证完成: %s（置信度 %d/10）。\n%s\n\n本子 Agent 任务结束。",
				findingID, status, confidence, reasonPreview))
			op.Exit()
		},
	)
}

func registerPhase3ReadReconNotesAction(r aicommon.AIInvokeRuntime, state *model.AuditState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
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
			summary, _ := reactloops.SpillLongContent(loop, "recon_notes", content)
			r.AddToTimeline("read_recon_notes", fmt.Sprintf("[Phase3] 读取项目背景报告 (%d 字节)", len(content)))
			op.Feedback(fmt.Sprintf("=== 项目背景报告 (%d bytes) ===\n\n%s", len(content), summary))
		},
	)
}
