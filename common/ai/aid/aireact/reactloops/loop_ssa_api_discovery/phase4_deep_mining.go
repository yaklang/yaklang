package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// runPhase4DeepMining runs per-endpoint ReAct deep mining (default Phase4 Step3).
func runPhase4DeepMining(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return nil
	}
	step := "phase4.step3.deep_mining"
	started := time.Now()
	rt.execStepStart(step, "ai+programmatic")
	_ = ctx
	targets, err := ListProbeTargets(rt)
	if err != nil {
		rt.execStepError(step, "ai+programmatic", started, err, nil)
		return err
	}
	step3Path := pl.GetStep3GreyboxReportPath()
	if len(targets) == 0 {
		reason := "无 verified=true 且 full_sample_url 非空的 probe target"
		if rt.Session != nil && rt.Session.TargetReachable {
			_ = recordPipelineWaiver(rt, 4, waiverDeepMiningNoTargets, reason)
		} else {
			_ = recordPipelineWaiver(rt, 4, waiverGreyboxSkipped, reason)
		}
		if r != nil {
			r.AddToTimeline("[ssa_phase4_step3]", "deep mining skipped: "+reason)
		}
		rt.execInfo(step, "programmatic", "skipped — "+reason)
		_ = writeDeepMiningSkippedReport(step3Path, reason)
		rt.execStepEnd(step, "ai+programmatic", started, []string{step3Path})
		return nil
	}
	for _, t := range targets {
		if err := runPhase4DeepMiningReAct(r, task, rt, pl, t); err != nil {
			log.Warnf("ssa_api_discovery: deep mining api=%d: %v", t.VerifiedHttpApiID, err)
			if r != nil {
				r.AddToTimeline("[ssa_phase4_step3]", fmt.Sprintf("deep mining failed api=%d: %v", t.VerifiedHttpApiID, err))
			}
		}
	}
	pl.SetGreyboxExecuted(true)
	if r != nil {
		r.AddToTimeline("[ssa_phase4_step3]", fmt.Sprintf("deep mining done targets=%d finalized=%d", len(targets), pl.CountDeepMiningDone()))
	}
	rt.execStepEnd(step, "ai+programmatic", started, []string{step3Path})
	return nil
}

// runPhase4Step3BatchScan runs legacy batch_scan greybox path.
func runPhase4Step3BatchScan(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, rt *Runtime, pl *PipelineState) error {
	step := "phase4.step3.batch_scan"
	started := time.Now()
	rt.execStepStart(step, "ai+programmatic")
	targets, _ := ListProbeTargets(rt)
	step3ReportPath := pl.GetStep3GreyboxReportPath()
	if len(targets) == 0 {
		if rt.Session != nil && rt.Session.TargetReachable {
			log.Errorf("ssa_api_discovery: phase4 step3 blocked: target reachable but no probe targets")
			if rt.Repo != nil {
				_ = rt.Repo.AppendEvent(rt.Session.ID, "error", "phase4_step3_no_targets", `{"step":"phase4_step3"}`)
			}
			if r != nil {
				r.AddToTimeline("[ssa_phase4_step3]", "greybox blocked: no probe targets while target reachable")
			}
			_ = recordPipelineWaiver(rt, 4, waiverDeepMiningNoTargets, "no probe targets")
		} else {
			_ = recordPipelineWaiver(rt, 4, waiverGreyboxSkipped, "no probe targets and target unreachable")
			if r != nil {
				r.AddToTimeline("[ssa_phase4_step3]", "greybox waived: no probe targets")
			}
			_ = writeDeepMiningSkippedReport(step3ReportPath, "靶机不可达或无 probe targets")
		}
		rt.execInfo(step, "programmatic", "skipped — no probe targets")
		rt.execStepEnd(step, "ai+programmatic", started, []string{step3ReportPath})
		return nil
	}
	step3Loop, err := buildPhase5Step3GreyboxLoop(r, rt, pl)
	if err != nil {
		rt.execStepError(step, "ai+programmatic", started, err, nil)
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanStart := time.Now()
	rt.execStepStart("phase4.step3.batch_scan.auto", "programmatic")
	if _, scanErr := RunVulnBatchScan(ctx, r, rt, DefaultVulnBatchScanParams()); scanErr != nil {
		rt.execStepError("phase4.step3.batch_scan.auto", "programmatic", scanStart, scanErr, nil)
		log.Warnf("ssa_api_discovery: phase4_step3 auto scan: %v", scanErr)
		if r != nil {
			r.AddToTimeline("[ssa_phase4_step3]", "greybox auto scan failed: "+scanErr.Error())
		}
	} else {
		rt.execStepEnd("phase4.step3.batch_scan.auto", "programmatic", scanStart, nil)
		pl.SetGreyboxExecuted(true)
	}
	if err := step3Loop.ExecuteWithExistedTask(newSubTask(task, "phase5_step3_greybox")); err != nil {
		log.Warnf("ssa_api_discovery: phase4_step3 react: %v", err)
	} else if !pl.GetGreyboxExecuted() {
		pl.SetGreyboxExecuted(true)
	}
	rt.execStepEnd(step, "ai+programmatic", started, []string{step3ReportPath})
	return nil
}
