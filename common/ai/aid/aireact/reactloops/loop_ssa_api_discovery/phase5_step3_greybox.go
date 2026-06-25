package loop_ssa_api_discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const step3GreyboxInstructionCore = `你是 **灰盒漏洞批量检测** 助手（Phase5 Step3，Congin 高效模式）。目标：对所有 HTTP 端点执行规则 payload 宽并发探测，命中后统一批量 AI 二次研判。

## 约束
- 每轮输出带 **@action** 的 JSON。
- 使用 **discovery_run_vuln_batch_scan** 启动嵌入式 Yak 工具（启发式 HTTP 模板 + 内置 payload 并发，AI 仅在后置批量研判阶段调用）。
- 检测完成后，使用 **discovery_list_dynamic_findings** 查看结果。
- 你可以对结果做最终审查，使用 **discovery_update_dynamic_finding** 调整状态/分析。
- 完成后 **directly_answer** 汇总：扫描端点数、发现漏洞数（按类型/严重度）、AI 审核数、http_phase_sec / total_elapsed_sec。
`

var step3GreyboxInstruction = strings.TrimSpace(step3GreyboxInstructionCore) + ssaDiscoveryReportLanguageZH +
	ssaDiscoveryDirectlyAnswerTitleBlock(4, "Phase4 Step3: 灰盒漏洞批量检测", "Step3")

func buildPhase5Step3GreyboxLoop(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) (*reactloops.ReActLoop, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}

	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	_ = os.MkdirAll(dir, 0o755)
	reportPath := filepath.Join(dir, "step3_greybox_scan.md")
	_ = os.WriteFile(reportPath, []byte(""), 0o644)
	pl.SetStep3GreyboxReportPath(reportPath)

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithPersistentInstruction(step3GreyboxInstruction),
		reactloops.WithPeriodicVerificationInterval(1000),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			base := ""
			if rt.Session != nil {
				base = EffectiveTargetBaseURL(rt.Session)
			}
			var credSummary string
			creds, _ := rt.Repo.ListVerifiedAuthCredentials(rt.Session.ID)
			if len(creds) > 0 {
				parts := make([]string, 0, len(creds))
				for _, c := range creds {
					parts = append(parts, fmt.Sprintf("id=%d type=%s header=%s", c.ID, c.AuthType, c.HeaderName))
				}
				credSummary = strings.Join(parts, "; ")
			} else {
				credSummary = "(no credentials)"
			}

			dynFindings, _ := rt.Repo.ListDynamicVulnFindings(rt.Session.ID)
			targets, _ := ListProbeTargets(rt)

			return fmt.Sprintf(`<|REACTIVE_STEP3_GREYBOX_%s|>
target_base: %s
sqlite: %s
auth_credentials: %s
probe_targets: %d
dynamic_findings: %d
step2_report: %s

feedback: %s
<|END_%s|>`,
				nonce, base, pl.SQLitePath,
				credSummary, len(targets), len(dynFindings),
				pl.GetStep2VerifyReportPath(),
				feedbacker.String(), nonce), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			loop.Set("discovery_session_uuid", rt.Session.UUID)
			loop.Set("discovery_sqlite_path", rt.SQLitePath)
			loop.Set("discovery_phase", rt.Session.Phase)
			if loop.Get("greybox_scan_done") != "1" {
				ctx := task.GetContext()
				if ctx == nil {
					ctx = loop.GetConfig().GetContext()
				}
				params := DefaultVulnBatchScanParams()
				if summary, err := RunVulnBatchScan(ctx, r, rt, params); err != nil {
					loop.GetInvoker().AddToTimeline("vuln_batch_scan_init", "auto scan failed: "+err.Error())
				} else {
					loop.Set("greybox_scan_done", "1")
					loop.GetInvoker().AddToTimeline("vuln_batch_scan_init", utils.ShrinkString(summary, 2000))
				}
			}
			op.Continue()
		}),
		buildDiscoveryGetStatus(),
		buildRunVulnBatchScanAction(r, rt, pl),
		buildListDynamicFindingsAction(),
		buildUpdateDynamicFindingAction(),
		buildPhaseDirectlyAnswerOverride(4, "Phase4 Step3: 灰盒漏洞批量检测", "Step3", false),
	}
	return reactloops.NewReActLoop("ssa_api_discovery_phase5_step3_greybox", r, preset...)
}

func buildRunVulnBatchScanAction(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_run_vuln_batch_scan",
		"Run the efficient vuln_batch_scan Yak tool: heuristic payloads with wide HTTP concurrency, batch AI secondary judgment at the end.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("resource_concurrent", aitool.WithParam_Default(8), aitool.WithParam_Description("endpoint concurrency")),
			aitool.WithIntegerParam("http_concurrent", aitool.WithParam_Default(16), aitool.WithParam_Description("payload HTTP concurrency")),
			aitool.WithIntegerParam("concurrent", aitool.WithParam_Description("legacy alias for http_concurrent")),
			aitool.WithIntegerParam("timeout", aitool.WithParam_Default(12)),
			aitool.WithIntegerParam("host_throttle_ms", aitool.WithParam_Default(0)),
			aitool.WithIntegerParam("auth_credential_id", aitool.WithParam_Description("credential id for auth; 0=auto-select")),
			aitool.WithStringParam("endpoint_ids", aitool.WithParam_Description("comma-separated endpoint IDs; empty=alive only")),
			aitool.WithStringParam("api_desc", aitool.WithParam_Description("JSON-encoded API descriptions for AI batch review")),
			aitool.WithIntegerParam("ai_concurrent", aitool.WithParam_Default(6), aitool.WithParam_Description("batch secondary judgment AI concurrency")),
			aitool.WithBoolParam("skip_ai_review", aitool.WithParam_Description("skip AI batch review, store keyword hits as uncertain")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			_, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			if EffectiveTargetBaseURL(sess) == "" {
				op.Feedback("target base URL not set")
				op.Continue()
				return
			}
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			params := DefaultVulnBatchScanParams()
			if v := action.GetInt("resource_concurrent"); v > 0 {
				params.ResourceConcurrent = v
			}
			if v := action.GetInt("http_concurrent"); v > 0 {
				params.HTTPConcurrent = v
			}
			if v := action.GetInt("concurrent"); v > 0 {
				params.HTTPConcurrent = v
			}
			params.Timeout = action.GetInt("timeout")
			params.HostThrottleMS = action.GetInt("host_throttle_ms")
			params.AIConcurrent = action.GetInt("ai_concurrent")
			params.AuthCredentialID = uint(action.GetInt("auth_credential_id"))
			params.EndpointIDs = action.GetString("endpoint_ids")
			params.APIDesc = action.GetString("api_desc")
			params.SkipAIReview = action.GetBool("skip_ai_review")
			summary, err := RunVulnBatchScan(ctx, r, rt, params)
			if err != nil {
				op.Feedback(fmt.Sprintf("vuln_batch_scan failed: %v", err))
				op.Continue()
				return
			}
			loop.Set("greybox_scan_done", "1")
			loop.GetInvoker().AddToTimeline("vuln_batch_scan", utils.ShrinkString(summary, 8000))
			op.Feedback(summary)
			op.Continue()
		},
	)
}

func buildListDynamicFindingsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_list_dynamic_findings",
		"List dynamic vulnerability findings from greybox scan.",
		[]aitool.ToolOption{
			aitool.WithStringParam("status_filter", aitool.WithParam_Description("confirmed|uncertain|false_positive; empty=all")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			statusFilter := strings.TrimSpace(action.GetString("status_filter"))
			var rows []store.DynamicVulnFinding
			var err error
			if statusFilter != "" {
				rows, err = rt.Repo.ListDynamicVulnFindingsByStatus(sess.ID, statusFilter)
			} else {
				rows, err = rt.Repo.ListDynamicVulnFindings(sess.ID)
			}
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			b, _ := json.MarshalIndent(rows, "", "  ")
			op.Feedback(utils.ShrinkString(string(b), 12000))
			op.Continue()
		},
	)
}

func buildUpdateDynamicFindingAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_update_dynamic_finding",
		"Update a dynamic finding status, AI analysis, and confidence after review.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("status", aitool.WithParam_Required(true), aitool.WithParam_Description("confirmed|uncertain|false_positive")),
			aitool.WithIntegerParam("confidence", aitool.WithParam_Description("0-100")),
			aitool.WithStringParam("ai_analysis"),
			aitool.WithStringParam("code_context"),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("id") <= 0 {
				return utils.Error("id required")
			}
			st := strings.ToLower(strings.TrimSpace(action.GetString("status")))
			valid := map[string]bool{"confirmed": true, "uncertain": true, "false_positive": true}
			if !valid[st] {
				return utils.Errorf("invalid status %q", action.GetString("status"))
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			fid := uint(action.GetInt("id"))
			row, err := rt.Repo.GetDynamicVulnFinding(sess.ID, fid)
			if err != nil {
				op.Feedback(fmt.Sprintf("finding not found: %v", err))
				op.Continue()
				return
			}
			row.Status = strings.ToLower(strings.TrimSpace(action.GetString("status")))
			if conf := action.GetInt("confidence"); conf > 0 {
				row.Confidence = int(conf)
			}
			if ana := action.GetString("ai_analysis"); ana != "" {
				row.AIAnalysis = ana
			}
			if cc := action.GetString("code_context"); cc != "" {
				row.CodeContext = cc
			}
			if err := rt.Repo.UpdateDynamicVulnFinding(row); err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if row.Status == "confirmed" {
				_ = BridgeDynamicFindingToVulnVerification(rt, row)
			}
			op.Feedback(fmt.Sprintf("updated dynamic_finding id=%d status=%s confidence=%d", fid, row.Status, row.Confidence))
			op.Continue()
		},
	)
}
