package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

func countHighPriorityChecklist(rt *Runtime) int {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0
	}
	items, err := rt.Repo.ListVulnChecklistItems(rt.Session.ID)
	if err != nil {
		return 0
	}
	n := 0
	for _, it := range items {
		if it.Priority >= highChecklistPriority {
			n++
		}
	}
	return n
}

func shouldSkipPhase4Step2StaticVerify(rt *Runtime) bool {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return true
	}
	findings, err := rt.Repo.ListDiscoverySyntaxFlowFindings(rt.Session.ID)
	if err != nil || len(findings) == 0 {
		return true
	}
	return countHighPriorityChecklist(rt) == 0
}

func hasStaticVerifySkippedWaiver(rt *Runtime) bool {
	if rt == nil || rt.Session == nil {
		return false
	}
	return hasPipelineWaiver(rt.Session, waiverStaticVerifySkipped)
}

// runPhase4Step2StaticVerify runs Step2 ReAct or skips with waiver + short report.
func runPhase4Step2StaticVerify(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	step := "phase4.step2.static_verify"
	started := time.Now()
	rt.execStepStart(step, "ai+programmatic")
	reportPath := pl.GetStep2VerifyReportPath()
	if shouldSkipPhase4Step2StaticVerify(rt) {
		body := `# [阶段 4/5 - Step2] Phase4 Step2: 静态发现动态验证

## 执行摘要

无 SyntaxFlow 静态发现或无可验证的高优先级 checklist 项，**本步已跳过**。

## 说明

- 不要求写入 vuln_verifications
- 进入 Step3 动态漏洞挖掘
`
		if err := os.WriteFile(reportPath, []byte(body), 0o644); err != nil {
			rt.execStepError(step, "ai+programmatic", started, err, []string{reportPath})
			return err
		}
		_ = recordPipelineWaiver(rt, 4, waiverStaticVerifySkipped, "no syntaxflow findings or high-priority checklist")
		if r != nil {
			r.AddToTimeline("[ssa_phase4_step2]", "skipped static verify (no high-priority checklist)")
		}
		rt.execInfo(step, "programmatic", "skipped — no high-priority checklist items")
		rt.execStepEnd(step, "ai+programmatic", started, []string{reportPath})
		return nil
	}

	step2Loop, err := buildPhase5Step2StaticVerifyLoop(r, rt, pl)
	if err != nil {
		rt.execStepError(step, "ai+programmatic", started, err, nil)
		return err
	}
	if err := step2Loop.ExecuteWithExistedTask(newSubTask(task, "phase5_step2_static_verify")); err != nil {
		log.Warnf("ssa_api_discovery: phase4_step2 react: %v", err)
	}
	backfillStart := time.Now()
	rt.execStepStart("phase4.step2.backfill", "programmatic")
	if n, berr := backfillStaticVulnVerifications(rt); berr != nil {
		rt.execStepError("phase4.step2.backfill", "programmatic", backfillStart, berr, nil)
		log.Warnf("ssa_api_discovery: phase4 step2 backfill: %v", berr)
	} else {
		rt.execStepEnd("phase4.step2.backfill", "programmatic", backfillStart, nil)
		if n > 0 && r != nil {
			r.AddToTimeline("[ssa_phase4_step2]", fmt.Sprintf("backfilled %d vuln_verifications", n))
		}
	}
	rt.execStepEnd(step, "ai+programmatic", started, []string{reportPath})
	return nil
}

// backfillStaticVulnVerifications writes safe rows for high-priority findings missing DB records.
func backfillStaticVulnVerifications(rt *Runtime) (int, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, nil
	}
	sid := rt.Session.ID
	checklist, err := rt.Repo.ListVulnChecklistItems(sid)
	if err != nil {
		return 0, err
	}
	verifications, err := rt.Repo.ListVulnVerifications(sid)
	if err != nil {
		return 0, err
	}
	done := map[uint]struct{}{}
	for _, v := range verifications {
		if v.SyntaxFlowFindingID > 0 {
			done[v.SyntaxFlowFindingID] = struct{}{}
		}
	}
	created := 0
	for _, item := range checklist {
		if item.Priority < highChecklistPriority {
			continue
		}
		if _, ok := done[item.FindingID]; ok {
			continue
		}
		row := &store.VulnVerification{
			SessionID:           sid,
			SyntaxFlowFindingID: item.FindingID,
			Source:              "syntaxflow",
			Status:              "safe",
			AIAnalysis:          "程序化回填：静态发现无外部 HTTP 触发面或 Step2 未落库（code_review_only）",
		}
		if err := rt.Repo.CreateVulnVerification(row); err != nil {
			return created, err
		}
		done[item.FindingID] = struct{}{}
		created++
	}
	return created, nil
}

func missingHighPriorityVulnVerifications(rt *Runtime) []uint {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	sid := rt.Session.ID
	checklist, _ := rt.Repo.ListVulnChecklistItems(sid)
	verifications, _ := rt.Repo.ListVulnVerifications(sid)
	done := map[uint]struct{}{}
	for _, v := range verifications {
		if v.SyntaxFlowFindingID > 0 {
			done[v.SyntaxFlowFindingID] = struct{}{}
		}
	}
	var missing []uint
	for _, item := range checklist {
		if item.Priority < highChecklistPriority {
			continue
		}
		if _, ok := done[item.FindingID]; !ok {
			missing = append(missing, item.FindingID)
		}
	}
	return missing
}

func buildStep2FinishGateWithVulnCoverage() reactloops.ReActLoopOption {
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType:  "finish",
		Description: "Finish Step2 only after all high-priority findings have vuln_verifications rows.",
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			rt := getRuntime(loop)
			if rt == nil {
				op.Exit()
				return
			}
			missing := missingHighPriorityVulnVerifications(rt)
			if len(missing) > 0 {
				op.Feedback(fmt.Sprintf("finish blocked: %d high-priority finding(s) missing discovery_upsert_vuln_verification: %v", len(missing), missing))
				op.Continue()
				return
			}
			op.Exit()
		},
	})
}
