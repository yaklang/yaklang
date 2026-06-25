package loop_ssa_api_discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const step2StaticVerifyInstructionCore = `你是 **静态发现动态验证** 助手（Phase5 Step2）。目标：对 Step0 待检清单中的静态发现，逐个或批量构造验证请求，判断漏洞是否真实存在。

## 约束
- 每轮输出带 **@action** 的 JSON。
- 先用 **discovery_list_syntaxflow_findings** 查看待验证项。
- 验证 URL 优先对照 **verified_http_apis.full_sample_url**（discovery_read_session_data entity=verified_http_apis）。
- 从 auth_credentials 中获取有效凭证，发包时自动附带鉴权 header（通过 auth_credential_id 参数）。
- 可用 **do_http_request** 发送单次验证请求，通过 auth_credential_id 自动注入鉴权。
- 也可用 **batch_verify_findings** 按漏洞类型批量并发验证多个端点。
- 验证策略：
  1. 先发正常请求作为 baseline
  2. 再发包含 payload 的请求
  3. 对比 baseline 与 payload 响应差异（状态码、响应体长度、错误信息）
  4. 根据差异判断 confirmed / safe / uncertain
- 对 uncertain 结果，尝试用不同 payload 重试 1-2 次。
- 结果写入 **discovery_upsert_vuln_verification**。
- **finish 前**：每条 priority≥3 的 checklist finding 必须已有 vuln_verifications 记录，否则 finish 会被拒绝。
- 完成后 **directly_answer** 汇总：验证总数、confirmed/safe/uncertain 统计。

## Payload 生成原则
- 根据静态发现的 data_flow_hint、matched_file 推断 sink 类型
- SQL注入: 根据参数类型选择字符串型/数字型 payload
- XSS: 使用无害标签 <xss> 或事件属性检测
- 命令注入: 使用时间延迟或回显型 payload
- 路径穿越: ../../etc/passwd 等标准路径
`

var step2StaticVerifyInstruction = strings.TrimSpace(step2StaticVerifyInstructionCore) + "\n\n" + strings.TrimSpace(ssaDiscoveryHTTPBuiltinToolParamsHint) +
	ssaDiscoveryReportLanguageZH + ssaDiscoveryDirectlyAnswerTitleBlock(4, "Phase4 Step2: 静态发现动态验证", "Step2")

func buildPhase5Step2StaticVerifyLoop(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) (*reactloops.ReActLoop, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}

	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	_ = os.MkdirAll(dir, 0o755)
	reportPath := filepath.Join(dir, "step2_static_verify.md")
	_ = os.WriteFile(reportPath, []byte(""), 0o644)
	pl.SetStep2VerifyReportPath(reportPath)

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(ssaDiscoveryMaxIterations(r)),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithPersistentInstruction(step2StaticVerifyInstruction),
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
					parts = append(parts, fmt.Sprintf("id=%d type=%s user=%s", c.ID, c.AuthType, c.Username))
				}
				credSummary = strings.Join(parts, "; ")
			} else {
				credSummary = "(无有效凭证)"
			}

			vv, _ := rt.Repo.ListVulnVerifications(rt.Session.ID)
			sfN := 0
			checklistN := 0
			if findings, err := rt.Repo.ListDiscoverySyntaxFlowFindings(rt.Session.ID); err == nil {
				sfN = len(findings)
			}
			if cl, err := rt.Repo.ListVulnChecklistItems(rt.Session.ID); err == nil {
				checklistN = len(cl)
			}

			return fmt.Sprintf(`<|REACTIVE_STEP2_VERIFY_%s|>
target_base: %s
sqlite: %s
checklist_entity: vuln_checklist_items (count=%d; use discovery_read_session_data)
auth_credentials: %s
syntaxflow_findings: %d
vuln_verifications: %d
discovery_report: %s

反馈: %s
<|END_%s|>`,
				nonce, base, pl.SQLitePath,
				checklistN,
				credSummary, sfN, len(vv),
				pl.GetDiscoveryReportPath(),
				feedbacker.String(), nonce), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			loop.Set("discovery_session_uuid", rt.Session.UUID)
			loop.Set("discovery_sqlite_path", rt.SQLitePath)
			loop.Set("discovery_phase", rt.Session.Phase)
			op.Continue()
		}),
		buildAuthAwareHTTPAction(r, rt, nil),
		buildDiscoveryReadSessionData(),
		buildDiscoveryGetStatus(),
		buildListSyntaxflowFindingsAction(),
		buildUpsertVulnVerification(),
		buildListAuthCredentialsAction(),
		buildSelectAuthCredentialAction(),
		buildBatchVerifyFindingsAction(r, rt),
		buildStep2FinishGateWithVulnCoverage(),
		buildPhaseDirectlyAnswerOverride(4, "Phase4 Step2: 静态发现动态验证", "Step2", false),
	}
	return reactloops.NewReActLoop("ssa_api_discovery_phase5_step2_verify", r, preset...)
}

// buildAuthAwareHTTPAction wraps do_http_request with optional auth_credential_id injection.
// When cfg.CalibrationRealm is set, successful login POSTs (302+Set-Cookie) are detected programmatically.
func buildAuthAwareHTTPAction(r aicommon.AIInvokeRuntime, rt *Runtime, cfg *AuthAwareHTTPActionConfig) reactloops.ReActLoopOption {
	calibrationRealm := ""
	var pinnedTarget *HttpProbeTarget
	if cfg != nil {
		calibrationRealm = cfg.CalibrationRealm
		pinnedTarget = cfg.PinnedTarget
	}
	return reactloops.WithRegisterLoopAction(
		"do_http_request",
		"HTTP 单次请求。可选 auth_credential_id 自动注入鉴权 header。",
		[]aitool.ToolOption{
			aitool.WithStringParam("url", aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("auth_credential_id", aitool.WithParam_Description("自动注入指定凭证的 header; 0=不注入")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			if rt != nil {
				if blocked := checkLoginPasswordTransformBlocked(rt, loop, calibrationRealm, action); blocked != "" {
					op.Feedback(blocked)
					op.Continue()
					return
				}
			}

			if calibrationRealm != "" {
				if blocked := checkLoginPOSTBlocked(rt, calibrationRealm, action); blocked != "" {
					op.Feedback(blocked)
					op.Continue()
					return
				}
			}

			params := extractLoopActionToolParams(action)
			params, paramNotes := augmentDoHTTPParams(params)
			params, pinNotes := pinHTTPParamsToProbeTarget(params, pinnedTarget)
			paramNotes = append(paramNotes, pinNotes...)

			credID := resolveHTTPAuthCredentialID(action.GetInt("auth_credential_id"), loop)
			var injectNotes []string
			var csrfNotes []string
			var cred *store.AuthCredential
			if credID > 0 && rt.Repo != nil && rt.Session != nil {
				var err error
				cred, err = rt.Repo.GetAuthCredential(rt.Session.ID, uint(credID))
				if err != nil {
					injectNotes = append(injectNotes, fmt.Sprintf("auth_credential_id=%d not found: %v", credID, err))
				} else {
					if _, _, ok := syncCsrfFromCredentialCookie(rt, cred); ok {
						injectNotes = append(injectNotes, "csrf synced from PUBLICCMS_ADMIN cookie")
					}
					csrfNotes = append(csrfNotes, stripManualCsrfFromParams(params, defaultCsrfParamName)...)
					injectNotes = append(injectNotes, applyAuthCredentialToHTTPParams(params, cred)...)
					requireCsrf := requiresCsrfForHTTPParams(rt, params)
					csrfNotes = append(csrfNotes, applyCachedCsrfForCredentialIfRequired(rt, uint(credID), params, requireCsrf)...)
				}
			} else if _, hadManual := stripManualAuthHeadersFromParams(params); hadManual {
				injectNotes = append(injectNotes, "manual Cookie/Authorization stripped or detected without auth_credential_id — set auth_credential_id via discovery_select_auth_credential")
			}

			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", params)
			if err != nil {
				op.Feedback(fmt.Sprintf("do_http_request failed: %v", err))
				op.Continue()
				return
			}
			content := toolResultTextContent(result)
			invoker.AddToTimeline("do_http_request", utils.ShrinkString(content, 4096))
			feedback := utils.ShrinkString(content, 12000)
			feedback += formatDoHTTPParamNormalizationHint(paramNotes)
			feedback += formatAuthInjectNotes(injectNotes)
			feedback += formatCsrfInjectNotes(csrfNotes)
			if cred != nil {
				if refreshNotes := refreshAuthCredentialFromHTTPResponse(rt, cred, content); len(refreshNotes) > 0 {
					feedback += formatAuthInjectNotes(refreshNotes)
				}
				if csrfMsg, _ := captureCsrfFromHTTPResponse(rt, cred, action.GetString("url"), content); csrfMsg != "" {
					feedback += csrfMsg
				}
			}
			feedback += enrichHTTPFeedbackWithLoginProbe(loop, rt, calibrationRealm, action, content)
			op.Feedback(feedback)
			op.Continue()
		},
	)
}

type batchVerifyResult struct {
	FindingID  uint   `json:"finding_id"`
	URL        string `json:"url"`
	StatusCode string `json:"status_code"`
	BaselineLen int   `json:"baseline_len"`
	PayloadLen  int   `json:"payload_len"`
	DiffHint    string `json:"diff_hint"`
	Error       string `json:"error,omitempty"`
}

func buildBatchVerifyFindingsAction(r aicommon.AIInvokeRuntime, rt *Runtime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"batch_verify_findings",
		"批量并发验证多个静态发现：对同类漏洞的多个端点并发发送 baseline + payload 请求，返回对比结果。",
		[]aitool.ToolOption{
			aitool.WithStringParam("finding_ids", aitool.WithParam_Required(true), aitool.WithParam_Description("逗号分隔的 syntaxflow_finding IDs")),
			aitool.WithStringParam("payload_template", aitool.WithParam_Required(true), aitool.WithParam_Description("payload 模板，{PARAM} 为注入位置占位符")),
			aitool.WithIntegerParam("auth_credential_id", aitool.WithParam_Description("自动注入的鉴权凭证 id; 0=不注入")),
			aitool.WithIntegerParam("concurrent", aitool.WithParam_Default(8)),
			aitool.WithIntegerParam("timeout", aitool.WithParam_Default(12)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			_, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}

			idsStr := action.GetString("finding_ids")
			payload := action.GetString("payload_template")
			conc := action.GetInt("concurrent")
			if conc <= 0 {
				conc = 8
			}

			var authHeader string
			if credID := action.GetInt("auth_credential_id"); credID > 0 && rt.Repo != nil {
				cred, err := rt.Repo.GetAuthCredential(sess.ID, uint(credID))
				if err == nil {
					SyncCredentialHeaderFields(cred)
					if cred.HeadersText != "" {
						authHeader = cred.HeadersText
					} else if cred.HeaderName != "" && cred.HeaderValue != "" {
						authHeader = fmt.Sprintf("%s: %s", cred.HeaderName, cred.HeaderValue)
					}
				}
			}

			base := ""
			if sess != nil {
				base = EffectiveTargetBaseURL(sess)
			}

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			ids := splitCSV(idsStr)
			checklistRows, _ := rt.Repo.ListVulnChecklistItems(sess.ID)
			checklistByFinding := map[uint]store.VulnChecklistItem{}
			for _, row := range checklistRows {
				checklistByFinding[row.FindingID] = row
			}

			type findingTarget struct {
				finding store.DiscoverySyntaxFlowFinding
				cl      store.VulnChecklistItem
				url     string
				epID    uint
			}
			var targets []findingTarget
			for _, idStr := range ids {
				fid, _ := strconv.Atoi(idStr)
				if fid <= 0 {
					continue
				}
				f, err := rt.Repo.GetDiscoverySyntaxFlowFinding(sess.ID, uint(fid))
				if err != nil {
					continue
				}
				cl := checklistByFinding[f.ID]
				targetURL := strings.TrimSpace(cl.FullSampleURL)
				if targetURL == "" && cl.EndpointID > 0 {
					targetURL = joinProbeURL(base, cl.PathPattern)
				}
				if targetURL == "" && cl.VerifiedHttpApiID > 0 {
					if vha, verr := rt.Repo.GetVerifiedHttpApi(sess.ID, cl.VerifiedHttpApiID); verr == nil && vha != nil {
						targetURL = strings.TrimSpace(vha.FullSampleURL)
						if targetURL == "" {
							targetURL = joinProbeURL(base, vha.PathPattern)
						}
					}
				}
				if targetURL == "" {
					targetURL = joinProbeURL(base, cl.PathPattern)
				}
				targets = append(targets, findingTarget{finding: *f, cl: cl, url: targetURL, epID: cl.EndpointID})
			}

			if len(targets) == 0 {
				op.Feedback("no valid findings matched to endpoints for batch verification")
				op.Continue()
				return
			}

			sem := make(chan struct{}, conc)
			var mu sync.Mutex
			var results []batchVerifyResult
			var wg sync.WaitGroup

			for _, t := range targets {
				wg.Add(1)
				go func(fe findingTarget) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					res := batchVerifyResult{FindingID: fe.finding.ID}
					if fe.cl.EndpointID == 0 && fe.cl.VerifiedHttpApiID == 0 && strings.TrimSpace(fe.url) == "" {
						res.Error = "no endpoint matched in vuln_checklist_items"
						mu.Lock()
						results = append(results, res)
						mu.Unlock()
						return
					}

					targetURL := fe.url
					res.URL = targetURL

					baseParams := aitool.InvokeParams{"url": targetURL}
					if authHeader != "" {
						baseParams["headers"] = authHeader
					}
					baseResult, _, berr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", baseParams)
					if berr != nil {
						res.Error = fmt.Sprintf("baseline: %v", berr)
						mu.Lock()
						results = append(results, res)
						mu.Unlock()
						return
					}
					baseContent := toolResultTextContent(baseResult)
					res.BaselineLen = len(baseContent)

					payloadURL := targetURL
					if strings.Contains(payloadURL, "?") {
						payloadURL += "&test=" + payload
					} else {
						payloadURL += "?test=" + payload
					}

					payParams := aitool.InvokeParams{"url": payloadURL}
					if authHeader != "" {
						payParams["headers"] = authHeader
					}
					payResult, _, perr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "do_http_request", payParams)
					if perr != nil {
						res.Error = fmt.Sprintf("payload: %v", perr)
						mu.Lock()
						results = append(results, res)
						mu.Unlock()
						return
					}
					payContent := toolResultTextContent(payResult)
					res.PayloadLen = len(payContent)

					lenDiff := res.PayloadLen - res.BaselineLen
					if lenDiff < 0 {
						lenDiff = -lenDiff
					}
					if lenDiff > 100 {
						res.DiffHint = fmt.Sprintf("response length diff: %d", lenDiff)
					} else {
						res.DiffHint = "responses similar"
					}

					mu.Lock()
					results = append(results, res)
					mu.Unlock()
				}(t)
			}
			wg.Wait()

			_ = payload // used in closure above
			b, _ := json.MarshalIndent(results, "", "  ")
			invoker.AddToTimeline("batch_verify_findings", utils.ShrinkString(string(b), 8000))
			op.Feedback(string(b))
			op.Continue()
		},
	)
}

func joinProbeURL(base, pathPattern string) string {
	p := strings.TrimSpace(pathPattern)
	if p == "" {
		return strings.TrimSuffix(strings.TrimSpace(base), "/")
	}
	lp := strings.ToLower(p)
	if strings.HasPrefix(lp, "http://") || strings.HasPrefix(lp, "https://") {
		return p
	}
	b := strings.TrimSuffix(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if b == "" {
		return p
	}
	return b + p
}

func buildListSyntaxflowFindingsAction() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_list_syntaxflow_findings",
		"List SyntaxFlow static findings for the current session.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("limit", aitool.WithParam_Default(200)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			rows, err := rt.Repo.ListDiscoverySyntaxFlowFindings(sess.ID)
			if err != nil {
				op.Feedback(fmt.Sprintf("list syntaxflow findings: %v", err))
				op.Continue()
				return
			}
			limit := action.GetInt("limit")
			if limit <= 0 {
				limit = 200
			}
			if len(rows) > limit {
				rows = rows[:limit]
			}
			raw, _ := json.MarshalIndent(rows, "", "  ")
			op.Feedback(string(raw))
			op.Continue()
		},
	)
}

func buildUpsertVulnVerification() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_upsert_vuln_verification",
		"Create or update a vuln verification row for a SyntaxFlow finding.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id", aitool.WithParam_Description("existing row id; 0=create")),
			aitool.WithIntegerParam("syntaxflow_finding_id", aitool.WithParam_Required(true)),
			aitool.WithStringParam("status", aitool.WithParam_Required(true), aitool.WithParam_Description("confirmed|safe|uncertain")),
			aitool.WithIntegerParam("confidence"),
			aitool.WithStringParam("exploit_payload"),
			aitool.WithStringParam("exploit_response"),
			aitool.WithStringParam("ai_analysis"),
			aitool.WithStringParam("fix"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, sess, ok := mustRT(loop, op)
			if !ok {
				return
			}
			fid := uint(action.GetInt("syntaxflow_finding_id"))
			if fid == 0 {
				op.Feedback("syntaxflow_finding_id required")
				op.Continue()
				return
			}
			status := strings.TrimSpace(action.GetString("status"))
			if status == "" {
				op.Feedback("status required")
				op.Continue()
				return
			}
			row := &store.VulnVerification{
				SessionID:           sess.ID,
				SyntaxFlowFindingID: fid,
				Source:              "syntaxflow",
				Status:              status,
				Confidence:          action.GetInt("confidence"),
				ExploitPayload:      action.GetString("exploit_payload"),
				ExploitResponse:     action.GetString("exploit_response"),
				AIAnalysis:          action.GetString("ai_analysis"),
				Fix:                 action.GetString("fix"),
			}
			if id := uint(action.GetInt("id")); id > 0 {
				row.ID = id
				if err := rt.Repo.UpdateVulnVerification(row); err != nil {
					op.Feedback(fmt.Sprintf("update vuln verification: %v", err))
					op.Continue()
					return
				}
			} else {
				if err := rt.Repo.CreateVulnVerification(row); err != nil {
					op.Feedback(fmt.Sprintf("create vuln verification: %v", err))
					op.Continue()
					return
				}
			}
			op.Feedback(fmt.Sprintf("saved vuln_verification id=%d finding=%d status=%s", row.ID, fid, status))
			op.Continue()
		},
	)
}
