package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

func finishPipelineAtMaxStage(r aicommon.AIInvokeRuntime, maxStage int, op *reactloops.InitTaskOperator) {
	r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("已在用户指定阶段 %d 结束（顺序完成阶段 1～%d，未跳步）", maxStage, maxStage))
	log.Infof("ssa_api_discovery: pipeline finished early at max stage %d", maxStage)
	op.Done()
}

func reloadRuntimeSession(rt *Runtime) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	s, err := rt.Repo.GetSessionByUUID(rt.Session.UUID)
	if err == nil && s != nil {
		rt.Session = s
	}
}

func enforceOrFail(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState, phaseIndex int, op *reactloops.InitTaskOperator) bool {
	if err := EnforcePhaseContract(rt, pl, phaseIndex); err != nil {
		failPipelineOnContract(r, rt, phaseIndex, err, op)
		return false
	}
	return true
}

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		PrepareSsaApiDiscoveryFocusMode(r)
		if c, ok := r.GetConfig().(*aicommon.Config); ok {
			mergeSsaDiscoveryIntervalReviewExtraPrompt(c)
		}

		parsed, err := ParseUserInputLenient(task.GetUserInput())
		if err != nil {
			op.Failed(err)
			return
		}
		mode, routeUsedLLM := ClassifySsaDiscoveryRoute(task.GetContext(), r, task.GetUserInput(), parsed)
		loop.Set("ssa_discovery_mode", mode)
		if routeUsedLLM {
			r.AddToTimeline("[ssa_discovery]", fmt.Sprintf("route=llm mode=%s", mode))
		} else {
			r.AddToTimeline("[ssa_discovery]", fmt.Sprintf("route=heuristic mode=%s", mode))
		}

	if mode == SsaDiscoveryModeQAReview {
		loop.Set("discovery_phase", "qa_review")
		op.Continue()
		return
	}

	workDir := discoveryTaskWorkDir(r)
	parsed, err = EnrichParsedForFullPipeline(task.GetContext(), r, task, workDir, parsed)
	if err != nil {
		op.Failed(err)
		return
	}

	userSetMaxStage := parsed.PipelineMaxStage > 0
	maxStage := NormalizePipelineMaxStage(parsed.PipelineMaxStage)
	if userSetMaxStage {
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("用户指定流水线执行至第 %d 阶段（顺序执行 1～%d，不跳步）", maxStage, maxStage))
	}
	log.Infof("ssa_api_discovery: pipeline max_stage=%d user_specified=%v", maxStage, userSetMaxStage)

	rt, err := BootstrapDiscoveryRuntimeFromParsed(r, task, parsed)
	if err != nil {
		op.Failed(err)
		return
	}
	defer func() { _ = closeGorm(rt.DB) }()

	startStage := ResolvePipelineStartStage(parsed, rt.Session)
	if startStage >= 6 {
		r.AddToTimeline("[ssa_pipeline]", "session 已完成全流程，跳过执行")
		op.Done()
		return
	}

	if rt.Session != nil && rt.Session.CodePathOK && startStage <= 1 {
		ctx := task.GetContext()
		if _, _, recErr := ReconcileSessionLanguageFromMarkers(ctx, rt); recErr != nil {
			log.Warnf("ssa_api_discovery: bootstrap language reconcile: %v", recErr)
		}
		reloadRuntimeSession(rt)
	}

	pl := NewPipelineState(rt.WorkDir, rt.SQLitePath, rt.Session.UUID)

	if parsed.PipelineResume || startStage > 1 {
		r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf(
			"checkpoint resume: start_stage=%d session_phase=%s",
			startStage, rt.Session.Phase,
		))
		log.Infof("ssa_api_discovery: resume start_stage=%d session=%s phase=%s", startStage, rt.Session.UUID, rt.Session.Phase)
	}

	log.Infof("ssa_api_discovery: pipeline start session=%s start_stage=%d", rt.Session.UUID, startStage)

	// Phase 1 — ReAct recon → staged code reading → catalog → auth sync → verify ReAct
	if startStage <= 1 || (userSetMaxStage && maxStage == 1) {
		phase1Start := time.Now()
		rt.execStepStart("phase1", "ai+programmatic")
		if err := runPhase1WithFrameworkToolkit(task.GetContext(), r, task, rt); err != nil {
			if IsPhase1AuthFailed(err) {
				rt.execStepError("phase1", "ai+programmatic", phase1Start, err, nil)
				r.AddToTimeline("[ssa_pipeline]", "phase1_auth_failed: "+err.Error())
				failPipelineOnContract(r, rt, 1, err, op)
				return
			}
			if IsPhase1VerificationGateFailed(err) {
				rt.execStepError("phase1", "ai+programmatic", phase1Start, err, nil)
				r.AddToTimeline("[ssa_pipeline]", "phase1_verify_gate_failed: "+err.Error())
				failPipelineOnContract(r, rt, 1, err, op)
				return
			}
			if IsPhase1BusinessCoverageFailed(err) {
				rt.execStepError("phase1", "ai+programmatic", phase1Start, err, nil)
				r.AddToTimeline("[ssa_pipeline]", "phase1_business_coverage_failed: "+err.Error())
				failPipelineOnContract(r, rt, 1, err, op)
				return
			}
			rt.execStepError("phase1", "ai+programmatic", phase1Start, err, nil)
			log.Warnf("ssa_api_discovery: phase1 redesigned error: %v", err)
		} else {
			rt.execStepEnd("phase1", "ai+programmatic", phase1Start, []string{
				store.Phase1DiscoveryReportPath(rt.WorkDir),
				store.FeatureInventoryPath(rt.WorkDir),
				store.DirectoryAnalysisPath(rt.WorkDir),
			})
		}
		reloadRuntimeSession(rt)
		EnsureHttpEndpointsIfEmpty(r, task.GetContext(), rt, "after_phase1")
		reloadRuntimeSession(rt)
		if !enforceOrFail(r, rt, pl, 1, op) {
			return
		}
		if err := finalizePhase1DiscoveryArtifacts(r, rt, pl); err != nil {
			log.Warnf("ssa_api_discovery: phase1 discovery artifacts: %v", err)
		}
		markSessionPhase(r, rt, PhaseApiVerified)
		emitPhaseSummary(r, rt, pl, collectPhase1BlockSummary(rt, time.Since(phase1Start), true, "Phase1: 技术架构 + 业务域 + 鉴权 + HTTP 验证", 0))
		if maxStage <= 1 {
			finishPipelineAtMaxStage(r, maxStage, op)
			return
		}
		} else {
			r.AddToTimeline("[ssa_pipeline]", "skip phase 1 (checkpoint)")
			reloadRuntimeSession(rt)
		}

		// Phase 2 — V-phase (feature work + api verification) + artifact checkpoint.
		// V-phase was moved here from runPhase1Redesigned so it runs in a fresh yak context
		// with an independent iteration budget, avoiding parent-task timeout.
	if startStage <= 2 {
	phase2Start := time.Now()
	rt.execStepStart("phase2", "ai+programmatic")
	if startStage > 1 {
		if err := ensurePhase1DiscoveryArtifacts(r, rt, pl); err != nil {
			log.Warnf("ssa_api_discovery: ensure discovery artifacts: %v", err)
		}
		reloadRuntimeSession(rt)
	}

	// V-phase is idempotent: if a valid coverage verdict already exists, skip re-running.
	vPhaseNeeded := true
	if shouldSkipVPhaseForToolkit(rt) {
		vPhaseNeeded = false
		rt.execInfo("phase2.v_phase", "programmatic", "skipped — framework_toolkit fast path")
	} else if decision, err := loadCoverageSignalDecision(rt); err == nil && decision != nil {
		verdict := strings.TrimSpace(string(decision.Verdict))
		if verdict == "finish" || verdict == "continue" || verdict == "reprioritize" {
			log.Infof("ssa_api_discovery: V-phase skipped — valid verdict %q already exists", verdict)
			vPhaseNeeded = false
		}
	}

	if vPhaseNeeded {
		// V-phase: run feature work chain + coverage verdict + gate.
		if err := runPhase1FeatureVerifyChain(task.GetContext(), r, task, rt); err != nil {
			rt.execStepError("phase2", "ai+programmatic", phase2Start, err, nil)
			r.AddToTimeline("[ssa_pipeline]", "phase1_v_phase: "+err.Error())
			if IsPhase1VerificationGateFailed(err) {
				failPipelineOnContract(r, rt, 2, err, op)
				return
			}
			log.Warnf("ssa_api_discovery: phase1 v-phase: %v", err)
		}
	} else {
		if shouldSkipVPhaseForToolkit(rt) {
			rt.execInfo("phase2.v_phase", "programmatic", "skipped — framework_toolkit fast path")
		} else {
			rt.execInfo("phase2.v_phase", "programmatic", "skipped — valid coverage verdict already exists")
		}
	}

	EnsureHttpEndpointsIfEmpty(r, task.GetContext(), rt, "after_phase1_v")
	reloadRuntimeSession(rt)

	// Build catalog from verified results
	catalogStart := time.Now()
	rt.execStepStart("phase2.assemble_api_catalog", "programmatic")
	if _, err := AssembleApiCatalogFromDB(rt); err != nil {
		if _, err2 := AssembleApiCatalogFromStages(rt); err2 != nil {
			rt.execStepError("phase2.assemble_api_catalog", "programmatic", catalogStart, err2, nil)
			log.Warnf("ssa_api_discovery: api_catalog: %v", err2)
		} else {
			rt.execStepEnd("phase2.assemble_api_catalog", "programmatic", catalogStart, []string{store.ApiCatalogPath(rt.WorkDir)})
		}
	} else {
		rt.execStepEnd("phase2.assemble_api_catalog", "programmatic", catalogStart, []string{store.ApiCatalogPath(rt.WorkDir)})
	}
	routeStart := time.Now()
	rt.execStepStart("phase2.route_candidates", "programmatic")
	if _, err := writeRouteCandidatesFromDB(rt); err != nil {
		rt.execStepError("phase2.route_candidates", "programmatic", routeStart, err, nil)
		log.Warnf("ssa_api_discovery: route_candidates: %v", err)
	} else {
		rt.execStepEnd("phase2.route_candidates", "programmatic", routeStart, []string{store.RouteCandidatesPath(rt.WorkDir)})
	}

	// Gate: verify coverage verdict + code-only + probe evidence
	gateStart := time.Now()
	rt.execStepStart("phase2.api_verification_gate", "programmatic")
	if err := RunPhase1FullApiVerificationGate(task.GetContext(), r, rt); err != nil {
		_ = WritePhase1DiscoveryReport(rt)
		rt.execStepError("phase2.api_verification_gate", "programmatic", gateStart, err, nil)
		rt.execStepError("phase2", "ai+programmatic", phase2Start, err, nil)
		r.AddToTimeline("[ssa_pipeline]", "phase1_gate_failed: "+err.Error())
		failPipelineOnContract(r, rt, 2, err, op)
		return
	}
	rt.execStepEnd("phase2.api_verification_gate", "programmatic", gateStart, nil)
	r.AddToTimeline("[ssa_pipeline]", "phase1_gate: ok "+refreshPhase1GateStatusLine(rt))

	artifactsStart := time.Now()
	rt.execStepStart("phase2.finalize_artifacts", "programmatic")
	if err := finalizePhase1DiscoveryArtifacts(r, rt, pl); err != nil {
		rt.execStepError("phase2.finalize_artifacts", "programmatic", artifactsStart, err, nil)
		log.Warnf("ssa_api_discovery: phase1 discovery artifacts: %v", err)
	} else {
		rt.execStepEnd("phase2.finalize_artifacts", "programmatic", artifactsStart, []string{
			store.Phase1DiscoveryReportPath(rt.WorkDir),
			store.FeatureInventoryPath(rt.WorkDir),
		})
	}
	reportStart := time.Now()
	rt.execStepStart("phase2.discovery_report", "programmatic")
	if err := WritePhase1DiscoveryReport(rt); err != nil {
		rt.execStepError("phase2.discovery_report", "programmatic", reportStart, err, nil)
		log.Warnf("ssa_api_discovery: phase1 report: %v", err)
	} else {
		rt.execStepEnd("phase2.discovery_report", "programmatic", reportStart, []string{store.Phase1DiscoveryReportPath(rt.WorkDir)})
	}
	markSessionPhase(r, rt, PhaseApiVerified)
	rt.execStepEnd("phase2", "ai+programmatic", phase2Start, []string{store.Phase1DiscoveryReportPath(rt.WorkDir), store.FeatureInventoryPath(rt.WorkDir)})
	emitPhaseSummary(r, rt, pl, collectPhase1BlockSummary(rt, time.Since(phase2Start), true, "Phase2: Feature Work + API Verification", 0))
		reloadRuntimeSession(rt)
		if !enforceOrFail(r, rt, pl, 2, op) {
			return
		}
		if maxStage <= 2 {
			finishPipelineAtMaxStage(r, maxStage, op)
			return
		}
		} else {
			r.AddToTimeline("[ssa_pipeline]", "skip phase 2 (checkpoint)")
			reloadRuntimeSession(rt)
		}

		// Phase 3 — SyntaxFlow 扫描（ReAct 子循环：AI 构造 filter → 执行扫描）
		if startStage <= 3 {
		phase3Start := time.Now()
		rt.execStepStart("phase3", "ai+programmatic")
		phase4EnsureSyntaxFlowScan(r, task, rt, pl)
		reloadRuntimeSession(rt)
		if !enforceOrFail(r, rt, pl, 3, op) {
			rt.execStepError("phase3", "ai+programmatic", phase3Start, utils.Error("phase3 contract failed"), []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
			return
		}
		markSessionPhase(r, rt, PhaseVulnScanned)
		rt.execStepEnd("phase3", "ai+programmatic", phase3Start, []string{store.SyntaxflowSummaryPath(rt.WorkDir)})
		emitPhaseSummary(r, rt, pl, collectPhase4Summary(rt, time.Since(phase3Start)))
		if maxStage <= 3 {
			finishPipelineAtMaxStage(r, maxStage, op)
			return
		}
		} else {
			r.AddToTimeline("[ssa_pipeline]", "skip phase 3 (checkpoint)")
			reloadRuntimeSession(rt)
		}

		// Phase 4 — 动态漏洞验证与检测（四步走）
		if startStage <= 4 {
		phase4Start := time.Now()
		rt.execStepStart("phase4", "ai+programmatic")
		runPhase5Pipeline(r, task, rt, pl)
		reloadRuntimeSession(rt)
		if !enforceOrFail(r, rt, pl, 4, op) {
			rt.execStepError("phase4", "ai+programmatic", phase4Start, utils.Error("phase4 contract failed"), []string{
				pl.GetStep0ReportPath(), pl.GetStep2VerifyReportPath(), pl.GetStep3GreyboxReportPath(),
			})
			return
		}
		markSessionPhase(r, rt, PhaseVulnVerified)
		rt.execStepEnd("phase4", "ai+programmatic", phase4Start, []string{
			pl.GetStep0ReportPath(), pl.GetStep1AuthReportPath(), pl.GetStep2VerifyReportPath(), pl.GetStep3GreyboxReportPath(),
		})
		if maxStage <= 4 {
			finishPipelineAtMaxStage(r, maxStage, op)
			return
		}
		} else {
			r.AddToTimeline("[ssa_pipeline]", "skip phase 4 (checkpoint)")
			reloadRuntimeSession(rt)
		}

		// Phase 5 — 最终报告
		if startStage <= 5 {
		phase5Start := time.Now()
		rt.execStepStart("phase5", "ai")
		phase5Err := runPhase6FinalReport(r, task, rt, pl)
		if phase5Err != nil {
			rt.execStepError("phase5", "ai", phase5Start, phase5Err, []string{pl.GetFinalReportPath()})
			log.Warnf("ssa_api_discovery: phase5 final report error: %v", phase5Err)
		} else {
			rt.execStepEnd("phase5", "ai", phase5Start, []string{pl.GetFinalReportPath()})
		}
		reloadRuntimeSession(rt)
		if !enforceOrFail(r, rt, pl, 5, op) {
			return
		}
		markSessionPhase(r, rt, PhasePipelineReport)
		} else {
			r.AddToTimeline("[ssa_pipeline]", "skip phase 5 (checkpoint)")
		}

		rt.execInfo("pipeline", "programmatic", fmt.Sprintf("completed; execution_log=%s", ExecutionLogPath(rt.WorkDir)))
		r.AddToTimeline("[ssa_pipeline]", "SSA API 全流程完成。discovery_report / syntaxflow_summary / final_audit_report 见 workdir/ssa_discovery/")
		op.Done()
	}
}

func markSessionPhase(r aicommon.AIInvokeRuntime, rt *Runtime, phase string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	rt.Session.Phase = phase
	if err := rt.Repo.UpdateSession(rt.Session); err != nil {
		log.Warnf("ssa_api_discovery: update phase %s: %v", phase, err)
		return
	}
	_ = rt.Repo.AppendEvent(rt.Session.ID, "info", "pipeline_phase", fmt.Sprintf(`{"phase":%q}`, phase))
	r.AddToTimeline("[ssa_pipeline]", fmt.Sprintf("phase=%s", phase))
}

// runPhase5Pipeline orchestrates the 4-step dynamic vulnerability verification pipeline (Phase 4).
func runPhase5Pipeline(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) {
	phase4Start := time.Now()
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	_ = os.MkdirAll(dir, 0o755)
	pl.SetStep0ReportPath(filepath.Join(dir, "step0_vuln_checklist.md"))
	pl.SetStep1AuthReportPath(filepath.Join(dir, "step1_auth_result.md"))
	pl.SetStep2VerifyReportPath(filepath.Join(dir, "step2_static_verify.md"))
	pl.SetStep3GreyboxReportPath(filepath.Join(dir, "step3_greybox_scan.md"))
	pl.SetPhase4Mode(rt.Phase4Mode())

	ctx := task.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}

	// Step 0 — summarize static findings, generate checklist
	runPhase5Step0Checklist(r, task, rt, pl)
	reloadRuntimeSession(rt)
	markSessionPhase(r, rt, PhaseStep0ChecklistDone)

	// Step 1 — sync Phase1 credentials (no ReAct re-login)
	if err := runPhase4Step1AuthSync(ctx, r, rt, pl); err != nil {
		log.Warnf("ssa_api_discovery: phase4_step1 sync: %v", err)
	}
	reloadRuntimeSession(rt)
	markSessionPhase(r, rt, PhaseStep1AuthDone)
	emitPhaseSummary(r, rt, pl, collectStep1Summary(rt))

	// Step 2 — static finding verification (skip / backfill / gate)
	if err := runPhase4Step2StaticVerify(ctx, r, task, rt, pl); err != nil {
		log.Warnf("ssa_api_discovery: phase4_step2: %v", err)
	}
	reloadRuntimeSession(rt)
	markSessionPhase(r, rt, PhaseStep2StaticDone)
	runPhase5StepReport(r, task, rt, pl, "step2_verify", "Phase4 Step2: 静态发现动态验证结果",
		`根据漏洞验证记录，撰写中文 Markdown 报告。
内容：
1. 验证总数及 confirmed/safe/uncertain 统计
2. 已确认漏洞清单（含端点、漏洞类型、证据摘要）
3. 不确定项清单（需人工复核）
4. 验证覆盖率分析`,
		"", pl.GetStep2VerifyReportPath())
	emitPhaseSummary(r, rt, pl, collectStep2Summary(rt))

	// Step 3 — deep mining (default) or legacy batch scan
	if rt.Phase4Mode() == Phase4ModeBatchScan {
		if err := runPhase4Step3BatchScan(ctx, r, task, rt, pl); err != nil {
			log.Warnf("ssa_api_discovery: phase4_step3 batch: %v", err)
		}
	} else {
		if err := runPhase4DeepMining(ctx, r, task, rt, pl); err != nil {
			log.Warnf("ssa_api_discovery: phase4_step3 deep mining: %v", err)
		}
	}
	reloadRuntimeSession(rt)
	bridgeStart := time.Now()
	rt.execStepStart("phase4.bridge_findings", "programmatic")
	if n, err := BridgeAllConfirmedDynamicFindings(rt); err != nil {
		rt.execStepError("phase4.bridge_findings", "programmatic", bridgeStart, err, nil)
		log.Warnf("ssa_api_discovery: bridge dynamic vulns: %v", err)
	} else {
		rt.execStepEnd("phase4.bridge_findings", "programmatic", bridgeStart, nil)
		if n > 0 {
			log.Infof("ssa_api_discovery: bridged %d dynamic findings to vuln_verifications", n)
		}
	}
	step3Title := "Phase4 Step3: 深度挖掘漏洞检测"
	step3Prompt := `根据 endpoint_vuln_probes / dynamic_vuln_findings，撰写中文 Markdown 报告。
内容：
1. 扫描端点数和每种 vuln_type 覆盖情况
2. 已确认漏洞详细清单（含 payload、证据）
3. 标记为 safe/skipped 的类型及原因
4. 与静态发现的交叉验证情况`
	if rt.Phase4Mode() == Phase4ModeBatchScan {
		step3Title = "Phase4 Step3: 灰盒漏洞批量检测结果"
		step3Prompt = `根据灰盒扫描发现数据，撰写中文 Markdown 报告。
内容：
1. 扫描端点数和检测到的漏洞数（按类型/严重度）
2. 已确认漏洞详细清单（含 payload、证据）
3. AI 误报过滤结果统计
4. 与静态发现的交叉验证情况`
	}
	runPhase5StepReport(r, task, rt, pl, "step3_greybox", step3Title, step3Prompt, "", pl.GetStep3GreyboxReportPath())
	emitPhaseSummary(r, rt, pl, collectStep3Summary(rt))

	emitPhaseSummary(r, rt, pl, collectPhase5Summary(rt, time.Since(phase4Start)))
}

// ExportDiscoverySnapshotJSON 写出当前会话的结构化摘要，供报告阶段读取。
func ExportDiscoverySnapshotJSON(rt *Runtime) (string, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return "", utils.Error("nil runtime")
	}
	sid := rt.Session.ID
	eps, _ := rt.Repo.ListHttpEndpoints(sid)
	comps, _ := rt.Repo.ListComponents(sid)
	secm, _ := rt.Repo.ListSecurityMechanisms(sid)
	vha, _ := rt.Repo.ListVerifiedHttpApis(sid)
	sf, _ := rt.Repo.ListDiscoverySyntaxFlowFindings(sid)
	vv, _ := rt.Repo.ListVulnVerifications(sid)
	ac, _ := rt.Repo.ListAuthCredentials(sid)
	df, _ := rt.Repo.ListDynamicVulnFindings(sid)
	checklist, _ := rt.Repo.ListVulnChecklistItems(sid)
	events, _ := rt.Repo.ListEvents(sid, 500)
	valAttempts, _ := rt.Repo.ListEndpointValidationAttemptsBySession(sid, 500)
	recipes, _ := rt.Repo.ListAuthAcquisitionRecipes(sid)
	configArts, _ := rt.Repo.ListConfigArtifacts(sid)
	deps, _ := rt.Repo.ListDependencies(sid)
	bizCaps, _ := rt.Repo.ListBusinessCapabilities(sid)
	artifacts, _ := rt.Repo.ListPhaseArtifacts(sid, "", 0)

	path := store.DiscoverySnapshotPath(rt.WorkDir)
	if err := store.WriteDiscoverySnapshot(path, rt.Session, eps, comps, secm, nil, vha, sf, vv, ac, df,
		checklist, nil, events, valAttempts, recipes, configArts, deps, bizCaps, artifacts); err != nil {
		return "", err
	}
	return path, nil
}

func closeGorm(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB := db.DB()
	if sqlDB != nil {
		return sqlDB.Close()
	}
	return nil
}
