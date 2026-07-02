package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/followup"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/persist"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/phase2"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/phase3"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/phase4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// BuildInitTask wires the four-phase audit pipeline into the root loop init task.
func BuildInitTask(r aicommon.AIInvokeRuntime, state *model.AuditState) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		log.Infof("[CodeAudit] Orchestrator started. workdir=%s phase=%s", state.WorkDir, state.GetPhase())
		userInput := task.GetUserInput()

		if state.GetPhase() == model.AuditPhaseDone {
			reactloops.EmitStatus(loop, "审计追问模式 / Audit follow-up mode")
			r.AddToTimeline("[AUDIT_FOLLOWUP]", "审计已完成，进入追问模式。用户输入: "+utils.ShrinkTextBlock(userInput, 300))
			followLoop, err := followup.BuildLoop(r, state)
			if err != nil {
				log.Errorf("[CodeAudit] Failed to build follow-up loop: %v", err)
				op.Failed(err)
				return
			}
			if err := followLoop.ExecuteWithExistedTask(task); err != nil {
				log.Warnf("[CodeAudit] Follow-up loop returned error: %v", err)
			}
			op.Done()
			return
		}

		reactloops.EmitStatus(loop, "代码安全审计启动 / Starting code security audit")
		r.AddToTimeline("[AUDIT_START]", "代码安全审计开始，用户输入: "+utils.ShrinkTextBlock(userInput, 300))

		ws := reactloops.InitWorkspaceAttachedContext(r, loop, task, AttachedResourceKeyCodeAuditTargetPath)
		reactloops.RecordWorkspaceAttachedTimeline(r, ws, "CODE_AUDIT")
		if ws != nil {
			var sel *aicommon.AttachedCodeSelection
			if ws.Selection != nil {
				copied := *ws.Selection
				sel = &copied
			}
			if ws.FilePath != "" || sel != nil {
				state.SetFrontendFocus(ws.FilePath, sel)
			}
		}

		auditDirPath := util.AuditDir(state)
		if err := os.MkdirAll(auditDirPath, 0o755); err != nil {
			log.Warnf("[CodeAudit] Failed to create audit dir %s: %v", auditDirPath, err)
			op.Failed(fmt.Sprintf("[CodeAudit] Fatal err failed to create audit dir %v", err))
			return
		}
		log.Infof("[CodeAudit] Audit dir ready: %s", auditDirPath)
		r.AddToTimeline("[AUDIT_DIR]", "审计输出目录: "+auditDirPath)

		runPhase1(loop, r, task, state, auditDirPath, ws, op)
		if failed, _ := op.IsFailed(); failed {
			return
		}

		runPhase2(loop, r, task, state, auditDirPath)

		runPhase3(loop, r, task, state, auditDirPath)

		runPhase4(loop, r, task, state, auditDirPath)

		reportPath := state.GetFinalReportPath()
		if reportPath == "" {
			reportPath = filepath.Join(auditDirPath, "security_audit_report.md")
		}
		finalReport := state.GetFinalReport()
		reactloops.EmitStatus(loop, "审计完成 / Audit complete")
		emit.PipelineDone(loop, reportPath, len(finalReport), state.GetStats())
		r.AddToTimeline("[AUDIT_DONE]", "代码安全审计全部完成。报告预览:\n"+utils.ShrinkTextBlock(finalReport, 200))
		log.Infof("[CodeAudit] All phases complete. Report length: %d bytes", len(finalReport))

		if err := persist.PersistToAuditDir(state, auditDirPath); err != nil {
			log.Warnf("[CodeAudit] Failed to persist audit state: %v", err)
		} else {
			log.Infof("[CodeAudit] Audit state persisted to %s", filepath.Join(auditDirPath, persist.AuditStateFileName))
		}

		op.Done()
	}
}

// runPhase1 leverage dir_explore loop to generate an outline for the target project
func runPhase1(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *model.AuditState, auditDirPath string, ws *reactloops.WorkspaceAttachedContext, op *reactloops.InitTaskOperator) {
	log.Infof("[CodeAudit] Starting Phase 1 (Recon via dir_explore)")
	reactloops.EmitStatus(loop, "Phase 1：项目探索中 / Phase 1: Project exploration...")
	r.AddToTimeline("[PHASE1_START]", "开始 Phase 1：项目探索（使用 dir_explore loop）")

	reconFilePath := filepath.Join(auditDirPath, "recon_notes.md")
	exploreOpts := []reactloops.ReActLoopOption{
		reactloops.WithVar("output_report_path", reconFilePath),
		reactloops.WithVar("explore_work_dir", auditDirPath),
	}
	scanPath := ""
	if ws != nil {
		scanPath = ws.ResolveAttachedScanDirectory()
	}
	emit.ReconStart(loop, scanPath)
	if scanPath != "" {
		if err := reactloops.ValidateAttachedDirectoryTarget(scanPath); err != nil {
			log.Warnf("[CodeAudit] attached target path not accessible: %q: %v", scanPath, err)
			op.Failed(fmt.Sprintf(
				"[CodeAudit] %s",
				reactloops.FormatAttachedDirectoryValidationError(scanPath, AttachedResourceKeyCodeAuditTargetPath, err)))
			return
		}
		exploreOpts = reactloops.WithExploreTargetPath(exploreOpts, scanPath)
		log.Infof("[CodeAudit] Phase1 using attached scan target: %s", scanPath)
		reactloops.RecordExploreTargetTimeline(r, ws, scanPath, "CODE_AUDIT")
	}
	exploreLoop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_DIR_EXPLORE, r, exploreOpts...)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to create dir_explore loop: %v", err)
		op.Failed(err)
		return
	}
	if err := exploreLoop.ExecuteWithExistedTask(util.NewSubTask(task, "phase1")); err != nil {
		log.Warnf("[CodeAudit] Phase 1 (dir_explore) returned error: %v (continuing)", err)
	}

	if projectPath := exploreLoop.Get("result_target_path"); projectPath != "" {
		projectName := exploreLoop.Get("result_project_name")
		if projectName == "" {
			projectName = filepath.Base(projectPath)
		}
		state.SetProjectInfo(projectPath, projectName)
	}
	techStack := exploreLoop.Get("result_tech_stack")
	entryPoints := exploreLoop.Get("result_entry_points")
	if techStack != "" {
		state.SetReconResult(techStack, entryPoints, "")
	}
	if reportPath := exploreLoop.Get("result_report_path"); reportPath != "" {
		state.SetReconFilePath(reportPath)
	}
	if noteFilesStr := exploreLoop.Get("result_note_files"); noteFilesStr != "" {
		for _, f := range strings.Split(noteFilesStr, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				state.AddReconNoteFile(f)
			}
		}
	}

	if state.TechStack == "" {
		log.Warnf("[CodeAudit] Phase 1 ended without tech_stack")
		r.AddToTimeline("[PHASE1_INCOMPLETE]", "警告：Phase 1 未完成探索就结束了。")
		emit.ReconComplete(loop, state.ProjectPath, "", state.GetReconFilePath(), true)
	} else {
		log.Infof("[CodeAudit] Phase 1 complete. tech=%s recon_file=%s", state.TechStack, state.GetReconFilePath())
		emit.ReconComplete(loop, state.ProjectPath, state.TechStack, state.GetReconFilePath(), false)
	}
	reactloops.EmitStatus(loop, "Phase 1 完成 / Phase 1 complete")
}

func runPhase2(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *model.AuditState, auditDirPath string) {
	log.Infof("[CodeAudit] Starting Phase 2")
	reactloops.EmitActionLog(loop, util.ScanNodeID, "Phase 2：代码审计扫描 / Phase 2: Code audit scan")
	reactloops.EmitStatus(loop, "Phase 2：漏洞扫描中 / Phase 2: Vulnerability scanning...")
	scanLoop, err := phase2.BuildAllCategoriesLoop(r, state, nil)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to build Phase 2 loop: %v", err)
		return
	}
	if err := scanLoop.ExecuteWithExistedTask(util.NewSubTask(task, "phase2")); err != nil {
		log.Warnf("[CodeAudit] Phase 2 returned error: %v (continuing)", err)
	}

	if len(state.GetFindings()) > 0 {
		findingsFile := filepath.Join(auditDirPath, "scan_findings.json")
		if err := state.PersistFindings(findingsFile); err != nil {
			log.Warnf("[CodeAudit] Failed to persist findings: %v", err)
		} else {
			r.AddToTimeline("[PHASE2_PERSISTED]", fmt.Sprintf("Phase 2 扫描完成，共 %d 个 finding 已写入: %s", len(state.GetFindings()), findingsFile))
		}
	}
	obsFile := filepath.Join(auditDirPath, "scan_observations.md")
	if err := state.PersistScanObservations(obsFile); err != nil {
		log.Warnf("[CodeAudit] Failed to persist scan_observations: %v", err)
	}
	reactloops.EmitStatus(loop, "Phase 2 完成 / Phase 2 complete")
}

func runPhase3(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *model.AuditState, auditDirPath string) {
	findings := state.GetFindings()
	if len(findings) == 0 {
		r.AddToTimeline("[NO_FINDINGS]", "扫描未发现疑似漏洞，跳过验证阶段。")
		emit.SkipVerify(loop)
		return
	}
	log.Infof("[CodeAudit] Starting Phase 3 (Verify), %d findings", len(findings))
	reactloops.EmitActionLog(loop, util.VerifyNodeID, "Phase 3：漏洞验证 / Phase 3: Vulnerability verification")
	r.AddToTimeline("[PHASE3_START]", "开始 Phase 3：逐 Finding 验证")
	verifyLoop, err := phase3.BuildVerifyLoop(r, state)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to build Phase 3 loop: %v", err)
		return
	}
	if err := verifyLoop.ExecuteWithExistedTask(util.NewSubTask(task, "phase3")); err != nil {
		log.Warnf("[CodeAudit] Phase 3 returned error: %v (continuing)", err)
	}
	state.DedupeVerifiedVulns()
	if filled := state.EnsureVerifyCoverage(); len(filled) > 0 {
		r.AddToTimeline("[PHASE3_GAP_FILL]", fmt.Sprintf("Phase3 补全 %d 个未验证 finding（uncertain）: %s", len(filled), strings.Join(filled, ", ")))
	}
	if len(state.GetFindings()) > 0 {
		verifiedFile := filepath.Join(auditDirPath, "verified_vulns.json")
		if err := state.PersistVerifiedVulns(verifiedFile); err != nil {
			log.Warnf("[CodeAudit] Failed to persist verified_vulns: %v", err)
		} else {
			r.AddToTimeline("[PHASE3_PERSISTED]", fmt.Sprintf("Phase 3 验证完成，共 %d 个结果: %s", len(state.GetVerifiedVulns()), verifiedFile))
		}
	}
	reactloops.EmitStatus(loop, "Phase 3 完成 / Phase 3 complete")
	emit.VerifyComplete(loop, "", state.GetStats())
}

func runPhase4(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *model.AuditState, auditDirPath string) {
	log.Infof("[CodeAudit] Starting Phase 4 (Report)")
	reactloops.EmitActionLog(loop, util.ReportNodeID, "Phase 4：审计报告 / Phase 4: Audit report")
	reactloops.EmitStatus(loop, "Phase 4：报告生成中 / Phase 4: Generating report...")
	r.AddToTimeline("[PHASE4_START]", "开始 Phase 4：报告生成")
	reportLoop, err := phase4.BuildReportLoop(r, state)
	if err != nil {
		log.Errorf("[CodeAudit] Failed to build Phase 4 loop: %v", err)
		return
	}
	if err := reportLoop.ExecuteWithExistedTask(util.NewSubTask(task, "phase4")); err != nil {
		log.Warnf("[CodeAudit] Phase 4 returned error: %v (continuing)", err)
	}
	if state.GetFinalReport() == "" {
		log.Warnf("[CodeAudit] Phase 4 did not produce report, generating fallback")
		fallbackReport := GenerateFallbackReport(state)
		state.SetFinalReport(fallbackReport)
		savePath := filepath.Join(auditDirPath, "security_audit_report.md")
		if err := reactloops.SaveAndPinFile(loop, savePath, []byte(fallbackReport)); err != nil {
			log.Warnf("[CodeAudit] Failed to write fallback report: %v", err)
		} else {
			state.SetFinalReportPath(savePath)
			r.AddToTimeline("[REPORT_FALLBACK]", "已自动生成基础审计报告: "+savePath)
			emit.ReportFallback(loop, savePath)
		}
	}
}
