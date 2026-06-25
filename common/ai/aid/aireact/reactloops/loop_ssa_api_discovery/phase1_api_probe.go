package loop_ssa_api_discovery

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/api_probe_playbook.txt
var apiProbePlaybook string

func buildPhase1ApiProbeLoop(r aicommon.AIInvokeRuntime, rt *Runtime, ctx *ProbeCandidateContext) (*reactloops.ReActLoop, error) {
	if rt == nil {
		return nil, utils.Error("nil runtime")
	}
	maxIter := ssaDiscoveryMaxIterations(r)
	if ctx != nil && ctx.MaxProbeIterations > 0 {
		maxIter = ctx.MaxProbeIterations
	}
	ctxJSON := ""
	if ctx != nil {
		b, _ := json.MarshalIndent(ctx, "", "  ")
		ctxJSON = string(b)
	}
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowToolCall(true),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithPersistentInstruction(
			strings.TrimSpace(apiProbePlaybook) + "\n\n" + strings.TrimSpace(ssaDiscoveryHTTPBuiltinToolParamsHint) +
				"\n\n## candidate_ctx\n```json\n" + ctxJSON + "\n```",
		),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			attempts := strings.TrimSpace(loop.Get("probe_attempts_log"))
			return fmt.Sprintf(`<|PROBE_REACTIVE_%s|>
candidate_id: %s
attempts_log:
%s

feedback:
%s
<|END_%s|>`, nonce, loop.Get("probe_candidate_id"), attempts, feedbacker.String(), nonce), nil
		}),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			setRuntime(loop, rt)
			if ctx != nil {
				loop.Set("probe_candidate_id", ctx.CandidateID)
			}
			loop.Set("probe_attempts_log", "")
			op.NextAction("do_http_request")
		}),
		buildDiscoveryGetStatus(),
		buildListAuthCredentialsAction(),
		buildSelectAuthCredentialAction(),
		buildAuthAwareHTTPAction(r, rt, nil),
		buildFinalizeProbeResult(),
		buildRecordProbeAttempt(),
		buildPhase1ProbeDirectlyAnswerOverride(),
		buildPhase1ProbeFinishOverride(),
	}
	return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY_API_PROBE, r, preset...)
}

func buildFinalizeProbeResult() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finalize_probe_result",
		"Submit final probe verdict JSON and end the probe sub-loop.",
		[]aitool.ToolOption{
			aitool.WithStringParam("probe_result_json", aitool.WithParam_Required(true), aitool.WithParam_Description("JSON: verified, method, path_pattern, full_sample_url, probe_attempts, verdict_reason, ...")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("probe_result_json")) == "" {
				return utils.Error("probe_result_json required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			raw := strings.TrimSpace(action.GetString("probe_result_json"))
			var pr ProbeResult
			if err := json.Unmarshal([]byte(raw), &pr); err != nil {
				op.Feedback("invalid probe_result_json: " + err.Error())
				op.Continue()
				return
			}
			if pr.Method == "" || pr.PathPattern == "" {
				op.Feedback("probe_result must include method and path_pattern")
				op.Continue()
				return
			}
			if pr.VerdictReason == "" {
				op.Feedback("verdict_reason required")
				op.Continue()
				return
			}
			if rt := getRuntime(loop); rt != nil {
				ApplyFailureSemanticsToProbeResult(rt, &pr)
			}
			b, _ := json.Marshal(pr)
			loop.Set("probe_final_result", string(b))
			loop.GetInvoker().AddToTimeline("finalize_probe_result", utils.ShrinkString(string(b), 2000))
			op.Exit()
		},
	)
}

func buildRecordProbeAttempt() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"record_probe_attempt",
		"Append one probe attempt summary to the reactive log.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("attempt", aitool.WithParam_Required(true)),
			aitool.WithStringParam("url", aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("status"),
			aitool.WithStringParam("verdict", aitool.WithParam_Required(true)),
			aitool.WithStringParam("note"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			line := fmt.Sprintf("attempt=%d url=%s status=%d verdict=%s %s\n",
				action.GetInt("attempt"), action.GetString("url"), action.GetInt("status"),
				action.GetString("verdict"), action.GetString("note"))
			prev := loop.Get("probe_attempts_log")
			loop.Set("probe_attempts_log", prev+line)
			op.Feedback("recorded")
			op.Continue()
		},
	)
}

func buildPhase1ProbeDirectlyAnswerOverride() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "directly_answer",
		Description: "Blocked in probe sub-loop; use finalize_probe_result instead.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			op.Feedback("probe sub-loop must end via finalize_probe_result, not directly_answer")
			op.Continue()
		},
	})
}

func buildPhase1ProbeFinishOverride() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "finish",
		Description: "Blocked until finalize_probe_result.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if strings.TrimSpace(loop.Get("probe_final_result")) == "" {
				op.Feedback("call finalize_probe_result before finish")
				op.Continue()
				return
			}
			op.Exit()
		},
	})
}

func buildDiscoveryProbeApiCandidate(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"discovery_probe_api_candidate",
		"Run loop_api_probe_react sub-loop for one API candidate. Pass candidate_ctx_json or discrete fields.",
		[]aitool.ToolOption{
			aitool.WithStringParam("candidate_ctx_json", aitool.WithParam_Description("Full ProbeCandidateContext JSON")),
			aitool.WithStringParam("method"),
			aitool.WithStringParam("path_pattern"),
			aitool.WithStringParam("handler_file"),
			aitool.WithStringParam("handler_symbol"),
			aitool.WithStringParam("handler_class"),
			aitool.WithStringParam("code_snippet"),
			aitool.WithIntegerParam("http_endpoint_id"),
			aitool.WithIntegerParam("max_probe_iterations"),
		},
		nil,
		func(parent *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt, _, ok := mustRT(parent, op)
			if !ok {
				return
			}
			ctx, err := probeCandidateContextFromAction(parent, action, rt)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if skip, reason := isProbeDestructivePath(ctx.PathPattern); skip {
				row := &store.VerifiedHttpApi{
					SessionID:     rt.Session.ID,
					Method:        ctx.Method,
					PathPattern:   ctx.PathPattern,
					HandlerFile:   ctx.HandlerFile,
					HandlerSymbol: ctx.HandlerSymbol,
					Verified:      false,
					RejectReason:  reason,
					Source:        "skipped_destructive",
					VerdictReason: reason,
				}
				if uerr := rt.Repo.UpsertVerifiedHttpApi(row); uerr != nil {
					op.Feedback("skip destructive endpoint failed: " + uerr.Error())
				} else {
					refreshPhase1VerifyLoopVars(parent, rt)
					op.Feedback(reason + " — pick next pending candidate via discovery_get_status")
				}
				op.Continue()
				return
			}
			spinKey := "phase1_probe_spin_" + routeKey(ctx.Method, ctx.PathPattern)
			spinCount := 0
			if v := strings.TrimSpace(parent.Get(spinKey)); v != "" {
				fmt.Sscanf(v, "%d", &spinCount)
			}
			spinCount++
			parent.Set(spinKey, fmt.Sprintf("%d", spinCount))
			if spinCount >= 3 {
				reason := fmt.Sprintf("probe_spin: auto-rejected after %d attempts without finalize_probe_result", spinCount)
				row := &store.VerifiedHttpApi{
					SessionID:    rt.Session.ID,
					Method:       ctx.Method,
					PathPattern:  ctx.PathPattern,
					HandlerFile:  ctx.HandlerFile,
					HandlerSymbol: ctx.HandlerSymbol,
					CodeSnippet:  ctx.CodeSnippet,
					Verified:     false,
					RejectReason: reason,
					Source:       "rejected",
					VerdictReason: reason,
				}
				if uerr := rt.Repo.UpsertVerifiedHttpApi(row); uerr != nil {
					op.Feedback("probe spin auto-reject failed: " + uerr.Error())
				} else {
					parent.GetInvoker().AddToTimeline("probe_spin_auto_reject",
						fmt.Sprintf("id=%d %s %s", row.ID, row.Method, row.PathPattern))
					refreshPhase1VerifyLoopVars(parent, rt)
					op.Feedback(reason + "; call discovery_get_status for next pending candidate — do not re-probe this route")
				}
				op.Continue()
				return
			}
			if strings.TrimSpace(ctx.CodeSnippet) == "" && strings.TrimSpace(ctx.HandlerFile) == "" {
				op.Feedback("probe blocked: no handler source after auto-load; ensure code_reading_plan has handler_file or static harvest file_rel_path")
				op.Continue()
				return
			}
			probeLoop, err := buildPhase1ApiProbeLoop(r, rt, ctx)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			task := parent.GetCurrentTask()
			if task == nil {
				op.Feedback("no current task")
				op.Continue()
				return
			}
			sub := newSubTask(task, "api_probe_"+ctx.CandidateID)
			if err := probeLoop.ExecuteWithExistedTask(sub); err != nil {
				op.Feedback("probe sub-loop error: " + err.Error())
				op.Continue()
				return
			}
			result := strings.TrimSpace(probeLoop.Get("probe_final_result"))
			if result == "" {
				op.Feedback("probe sub-loop ended without probe_final_result")
				op.Continue()
				return
			}
			var pr ProbeResult
			if err := json.Unmarshal([]byte(result), &pr); err == nil {
				if pr.HandlerFile == "" {
					pr.HandlerFile = ctx.HandlerFile
				}
				if pr.HandlerSymbol == "" {
					pr.HandlerSymbol = ctx.HandlerSymbol
				}
				if pr.CodeSnippet == "" {
					pr.CodeSnippet = ctx.CodeSnippet
				}
				if pr.EffectiveBase == "" && len(ctx.EffectiveBases) > 0 {
					pr.EffectiveBase = ctx.EffectiveBases[0]
				}
				ApplyFailureSemanticsToProbeResult(rt, &pr)
				if row, uerr := UpsertVerifiedHttpApiFromProbeResult(rt, &pr); uerr != nil {
					parent.GetInvoker().AddToTimeline("probe_upsert_verified_http_api", uerr.Error())
				} else {
					parent.GetInvoker().AddToTimeline("probe_upsert_verified_http_api",
						fmt.Sprintf("id=%d verified=%v method=%s path=%s", row.ID, row.Verified, row.Method, row.PathPattern))
				}
			}
			parent.Set("last_probe_result", result)
			parent.Set(spinKey, "0")
			refreshPhase1VerifyLoopVars(parent, rt)
			op.Feedback("probe_result:\n" + utils.ShrinkString(result, 12000))
			op.Continue()
		},
	)
}

func probeCandidateContextFromAction(parent *reactloops.ReActLoop, action *aicommon.Action, rt *Runtime) (*ProbeCandidateContext, error) {
	raw := strings.TrimSpace(action.GetString("candidate_ctx_json"))
	if raw != "" {
		var ctx ProbeCandidateContext
		if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
			return nil, err
		}
		enrichProbeCandidateContext(rt, &ctx)
		return &ctx, nil
	}
	method := action.GetString("method")
	path := action.GetString("path_pattern")
	if method == "" || path == "" {
		return nil, utils.Error("method and path_pattern or candidate_ctx_json required")
	}
	cid := fmt.Sprintf("ep-%d", action.GetInt("http_endpoint_id"))
	if action.GetInt("http_endpoint_id") <= 0 {
		cid = fmt.Sprintf("%s-%s", method, path)
	}
	snippet := action.GetString("code_snippet")
	if snippet == "" {
		snippet = parent.Get("phase1_code_snippet")
	}
	hf := action.GetString("handler_file")
	if hf == "" {
		hf = parent.Get("phase1_handler_file")
	}
	hs := action.GetString("handler_symbol")
	if hs == "" {
		hs = parent.Get("phase1_handler_symbol")
	}
	maxIter := action.GetInt("max_probe_iterations")
	if maxIter <= 0 {
		if parent != nil {
			if inv := parent.GetInvoker(); inv != nil {
				maxIter = maxIterationsFromConfig(inv.GetConfig())
			}
		}
		if maxIter <= 0 {
			maxIter = defaultReActMaxIterations
		}
	}
	authSurf := ""
	if b, err := osReadFileShrink(store.AuthSurfacePath(rt.WorkDir), 4000); err == nil {
		authSurf = b
	}
	handlerClass := action.GetString("handler_class")
	if epID := action.GetInt("http_endpoint_id"); epID > 0 && rt.Repo != nil && rt.Session != nil {
		if ep, err := rt.Repo.GetHttpEndpoint(rt.Session.ID, uint(epID)); err == nil && ep != nil {
			if handlerClass == "" {
				handlerClass = ep.HandlerClass
			}
			if hf == "" && ep.HandlerClass != "" {
				hf = guessFileFromHandlerClass(ep.HandlerClass)
			}
			if hs == "" {
				hs = ep.HandlerMethod
			}
		}
	}
	ctx := &ProbeCandidateContext{
		CandidateID:        cid,
		Method:             method,
		PathPattern:        path,
		HandlerClass:       handlerClass,
		HandlerFile:        hf,
		HandlerSymbol:      hs,
		CodeSnippet:        snippet,
		EffectiveBases:     parseEffectiveBasesFromSession(rt),
		MaxProbeIterations: maxIter,
		AuthSurfaceJSON:    authSurf,
	}
	enrichProbeCandidateContext(rt, ctx)
	return ctx, nil
}

func enrichProbeContextFromPlan(rt *Runtime, ctx *ProbeCandidateContext) {
	if rt == nil || ctx == nil {
		return
	}
	plan, err := LoadCodeReadingPlanForRuntime(rt)
	if err != nil || plan == nil {
		return
	}
	api := LookupDiscoveredAPI(plan, ctx.Method, ctx.PathPattern)
	if api == nil {
		return
	}
	if ctx.HandlerFile == "" {
		ctx.HandlerFile = api.HandlerFile
	}
	if ctx.HandlerSymbol == "" {
		ctx.HandlerSymbol = api.HandlerSymbol
	}
	if ctx.CodeSnippet == "" && api.CodeEvidence != "" && !isWeakCodeEvidence(api.CodeEvidence) {
		ctx.CodeSnippet = api.CodeEvidence
	}
	if ctx.HandlerClass == "" {
		ctx.HandlerClass = strings.TrimSpace(api.HandlerClass)
	}
}

func osReadFileShrink(path string, max int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return utils.ShrinkString(string(b), max), nil
}
