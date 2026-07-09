package phase2

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/subagent"
	"github.com/yaklang/yaklang/common/log"
)

const defaultCategoryScanConcurrency = 3

type categoryScanJob struct {
	category model.VulnCategory
	index    int
	total    int
}

type categoryScanOutcome struct {
	category     model.VulnCategory
	index        int
	incomplete   bool
	findingCount int
	execErr      error
}

// runAllCategoryScans executes Phase2 category scans via forked sub-agents (timeline isolation, optional parallelism).
func runAllCategoryScans(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
	categories []model.VulnCategory,
) []categoryScanOutcome {
	if len(categories) == 0 {
		return nil
	}

	jobs := make([]subagent.ForkJob, 0, len(categories))
	catalog := make(map[string]categoryScanJob, len(categories))
	for i, category := range categories {
		jobs = append(jobs, subagent.ForkJob{
			Order:      i + 1,
			Identifier: category.ID,
			Goal:       fmt.Sprintf("Phase 2 category scan: %s (%s)", category.Name, category.ID),
		})
		catalog[category.ID] = categoryScanJob{
			category: category,
			index:    i + 1,
			total:    len(categories),
		}
	}

	concurrency := defaultCategoryScanConcurrency
	if len(categories) < concurrency {
		concurrency = len(categories)
	}

	log.Infof("[CodeAudit/Phase2] Starting forked sub-agent scan of %d categories (concurrency=%d)", len(categories), concurrency)
	r.AddToTimeline("[PHASE2_START]",
		fmt.Sprintf("Phase 2 开始：fork 子 Agent 扫描 %d 个漏洞类别（并发 %d，timeline 分支隔离）。", len(categories), concurrency))

	artifacts := newCategoryArtifactStore(state)

	var scanStates sync.Map

	forkResults := subagent.RunForkJobsConcurrently(
		r, task, jobs, concurrency,
		func(childInvoker aicommon.AIInvokeRuntime, job subagent.ForkJob) (*reactloops.ReActLoop, error) {
			catJob, ok := catalog[job.Identifier]
			if !ok {
				return nil, fmt.Errorf("unknown category job %q", job.Identifier)
			}
			catLoop, scan, err := buildSingleCategoryScanLoop(
				childInvoker, state, catJob.category, catJob.index, catJob.total, nil, artifacts,
			)
			if scan != nil {
				scanStates.Store(catJob.category.ID, scan)
			}
			return catLoop, err
		},
	)

	sort.Slice(forkResults, func(i, j int) bool {
		return forkResults[i].Order < forkResults[j].Order
	})

	outcomes := make([]categoryScanOutcome, 0, len(forkResults))
	for _, forkResult := range forkResults {
		if forkResult == nil {
			continue
		}
		catJob, ok := catalog[forkResult.Identifier]
		if !ok {
			continue
		}
		var scanState *ScanState
		if raw, ok := scanStates.Load(catJob.category.ID); ok {
			scanState, _ = raw.(*ScanState)
		}
		outcome := finalizeCategoryScanAfterFork(r, loop, task, state, catJob, forkResult, scanState, &scanStates, artifacts)
		outcomes = append(outcomes, outcome)

		emit.Phase2CategoryOutcome(loop, catJob.index, catJob.total, catJob.category, outcome.findingCount, outcome.incomplete)
		log.Infof("[CodeAudit/Phase2] [%d/%d] Category '%s' complete. Total findings so far: %d",
			catJob.index, catJob.total, catJob.category.ID, len(state.GetFindings()))
	}

	auditDirPath := util.AuditDir(state)
	if err := artifacts.MergeAll(auditDirPath); err != nil {
		log.Warnf("[CodeAudit/Phase2] Failed to merge phase2 artifacts: %v", err)
	}

	return outcomes
}

func finalizeCategoryScanAfterFork(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
	catJob categoryScanJob,
	forkResult *subagent.ForkResult,
	scanState *ScanState,
	scanStates *sync.Map,
	artifacts *categoryArtifactStore,
) categoryScanOutcome {
	category := catJob.category
	if forkResult.ExecErr != nil {
		log.Warnf("[CodeAudit/Phase2] Category '%s' forked sub-agent error: %v", category.ID, forkResult.ExecErr)
	}

	obsBefore := countScanObservationsForCategory(state, category.ID)
	incomplete := obsBefore == 0

	if incomplete && tryResumeCategoryScanPhaseB(r, loop, task, state, catJob, scanState, scanStates, artifacts) {
		incomplete = countScanObservationsForCategory(state, category.ID) == 0
	}

	if incomplete {
		log.Warnf("[CodeAudit/Phase2] Category '%s' ended without calling complete_scan.", category.ID)
		msg := fmt.Sprintf("类别 '%s' 扫描未调用 complete_scan 就结束了，可能未完整审计。", category.ID)
		r.AddToTimeline("[PHASE2_CAT_INCOMPLETE]", "警告："+msg)
		emit.Phase2ScanWarning(loop, category, "incomplete", msg)
	}

	findingCount := 0
	for _, f := range state.GetFindings() {
		if f.Category == category.ID {
			findingCount++
		}
	}

	return categoryScanOutcome{
		category:     category,
		index:        catJob.index,
		incomplete:   incomplete,
		findingCount: findingCount,
		execErr:      forkResult.ExecErr,
	}
}

// shouldResumeCategoryScanFromPhaseA returns true when a category sub-agent ended
// stuck in phase A but already has locked targets or fast_context discovery candidates.
func shouldResumeCategoryScanFromPhaseA(scanState *ScanState) bool {
	if scanState == nil || scanState.CurrentPhase() != ScanPhaseSearch {
		return false
	}
	return scanState.TargetFileCount() > 0 || scanState.DiscoveryCandidateCount() > 0
}

func tryResumeCategoryScanPhaseB(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
	catJob categoryScanJob,
	scanState *ScanState,
	scanStates *sync.Map,
	artifacts *categoryArtifactStore,
) bool {
	category := catJob.category
	if !shouldResumeCategoryScanFromPhaseA(scanState) {
		return false
	}

	autoLocked, skipped := scanState.PrepareDiscoveryGateForPhaseB()
	locked := scanState.CommitToAudit()
	if len(locked) == 0 {
		return false
	}

	log.Warnf("[CodeAudit/Phase2] Category '%s' stuck in phase A; resuming phase B (targets=%d auto_locked=%d skipped=%d)",
		category.ID, len(locked), autoLocked, skipped)
	r.AddToTimeline("[PHASE2_RESUME]",
		fmt.Sprintf("[Phase2/%s] 阶段A 提前结束，已自动纳入候选并恢复阶段B（%d 个目标）",
			category.ID, len(locked)))
	emit.Phase2ScanWarning(loop, category, "resume_phase_b",
		fmt.Sprintf("阶段A 未完成，已从 %d 个目标恢复阶段B", len(locked)))

	resumeJob := subagent.ForkJob{
		Order:      catJob.index,
		Identifier: category.ID + "-resume",
		Goal:       fmt.Sprintf("Phase 2 category scan resume: %s", category.Name),
	}
	resumeResult, resumeErr := subagent.RunForkJob(r, task, resumeJob, func(childInvoker aicommon.AIInvokeRuntime, _ subagent.ForkJob) (*reactloops.ReActLoop, error) {
		catLoop, scan, err := buildSingleCategoryScanLoop(childInvoker, state, category, catJob.index, catJob.total, scanState, artifacts)
		if scan != nil {
			scanStates.Store(category.ID, scan)
		}
		return catLoop, err
	})
	if resumeErr != nil {
		log.Warnf("[CodeAudit/Phase2] Category '%s' resume fork failed: %v", category.ID, resumeErr)
		return false
	}
	if resumeResult != nil && resumeResult.ExecErr != nil {
		log.Warnf("[CodeAudit/Phase2] Category '%s' resume scan failed: %v", category.ID, resumeResult.ExecErr)
	}
	return true
}

func countScanObservationsForCategory(state *model.AuditState, categoryID string) int {
	count := 0
	for _, obs := range state.GetScanObservations() {
		if obs != nil && obs.CategoryID == categoryID {
			count++
		}
	}
	return count
}
