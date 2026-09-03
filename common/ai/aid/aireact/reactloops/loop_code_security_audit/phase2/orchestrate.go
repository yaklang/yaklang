package phase2

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/auditopts"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

type categoryScanJob struct {
	category model.VulnCategory
	index    int
	total    int
}

type categoryScanOutcome struct {
	category     model.VulnCategory
	index        int
	total        int
	incomplete   bool
	findingCount int
	execErr      error
}

// pendingResume 是一个被中断、等待批量恢复续扫的类别条目。
type pendingResume struct {
	catJob    categoryScanJob
	scanState *ScanState
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

	jobs := make([]reactloops.SubAgentJob, 0, len(categories))
	catalog := make(map[string]categoryScanJob, len(categories))
	for i, category := range categories {
		goal := fmt.Sprintf("Phase 2 category scan: %s (%s)", category.Name, category.ID)
		jobs = append(jobs, reactloops.SubAgentJob{
			Order:      i + 1,
			Identifier: category.ID,
			TaskName:   goal,
			Goal:       goal,
			LoopName:   schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT,
			Timeout:    auditopts.DefaultCategoryScanTimeout,
		})
		catalog[category.ID] = categoryScanJob{
			category: category,
			index:    i + 1,
			total:    len(categories),
		}
	}

	concurrency := reactloops.ResolveSubAgentConcurrency(loop.GetMaxSubAgents(), len(categories))

	log.Infof("[CodeAudit/Phase2] Starting forked sub-agent scan of %d categories (concurrency=%d, per-category timeout=%s)",
		len(categories), concurrency, auditopts.DefaultCategoryScanTimeout)
	r.AddToTimeline("[PHASE2_START]",
		fmt.Sprintf("Phase 2 开始：fork 子 Agent 扫描 %d 个漏洞类别（timeline 分支隔离，每类超时 %s）。",
			len(categories), auditopts.DefaultCategoryScanTimeout))

	artifacts := newCategoryArtifactStore(state)

	var scanStates sync.Map

	forkResults := reactloops.DispatchSubAgents(r, task, jobs, reactloops.SubAgentOptions{
		ParentLoop:         loop,
		TimelineMode:       reactloops.SubAgentTimelineFork,
		ExecuteConcurrency: concurrency,
		DefaultJobTimeout:  auditopts.DefaultCategoryScanTimeout,
		ExtraConfigOpts:    auditopts.SubAgentConfigOpts(),
		LoopBuilder: phase2CategoryLoopBuilder{
			state: state, catalog: catalog, artifacts: artifacts, scanStates: &scanStates,
		},
	})

	sort.Slice(forkResults, func(i, j int) bool {
		return forkResults[i].Order < forkResults[j].Order
	})

	// ── 收尾三段式 ──
	// 1) 评估：逐类算 incomplete，判定可恢复类（阶段A/阶段B中断）并做阶段A的
	//    gate+commit 预处理；不可恢复且无 observation 的当场走兜底。
	// 2) 批量 resume：所有可恢复类合并成一次 DispatchSubAgents（同主池并发度，
	//    并行续扫），避免逐类串行 resume 把 Phase2 尾部拖成 8×75min。
	// 3) 兜底复查：resume 后仍无 observation 且未完成的类别执行 auto-finalize
	//    兜底，保证每个类别在 Phase2 结束时必有 observation（Phase3/4 依赖）。
	outcomes := make([]categoryScanOutcome, 0, len(forkResults))
	resumables := make([]pendingResume, 0, len(forkResults))
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

		category := catJob.category
		if forkResult.ExecErr != nil {
			log.Warnf("[CodeAudit/Phase2] Category '%s' forked sub-agent error: %v", category.ID, forkResult.ExecErr)
		}

		incomplete := countScanObservationsForCategory(state, category.ID) == 0
		if incomplete && prepareCategoryResume(scanState, category.ID) {
			log.Infof("[CodeAudit/Phase2] Category '%s' interrupted; queued for phase-B resume.", category.ID)
			r.AddToTimeline("[PHASE2_RESUME_QUEUED]",
				fmt.Sprintf("[Phase2/%s] 扫描被中断但可恢复，已加入批量恢复队列。", category.ID))
			resumables = append(resumables, pendingResume{catJob: catJob, scanState: scanState})
			continue
		}
		if incomplete {
			// 不可恢复（无 scanState / 无目标 / 阶段A空手 / 阶段B已全部mark）：
			// 直接兜底，保证该类别在 Phase2 结束时有 observation 供 Phase3/4 消费。
			fallbackFinalizeCategoryScan(r, state, scanState, category, forkResult.ExecErr, artifacts)
		}
		outcomes = append(outcomes, buildCategoryScanOutcome(state, catJob, forkResult.ExecErr))
	}

	if len(resumables) > 0 {
		resumeJobs := make([]reactloops.SubAgentJob, 0, len(resumables))
		resumeCatalog := make(map[string]pendingResume, len(resumables))
		for _, p := range resumables {
			resumeGoal := fmt.Sprintf("Phase 2 category scan resume: %s", p.catJob.category.Name)
			identifier := p.catJob.category.ID + "-resume"
			resumeJobs = append(resumeJobs, reactloops.SubAgentJob{
				Order:      p.catJob.index,
				Identifier: identifier,
				TaskName:   resumeGoal,
				Goal:       resumeGoal,
				Timeout:    auditopts.DefaultCategoryScanTimeout,
			})
			resumeCatalog[identifier] = p
		}
		log.Infof("[CodeAudit/Phase2] Resuming %d interrupted categories in parallel (timeout=%s each).",
			len(resumables), auditopts.DefaultCategoryScanTimeout)
		r.AddToTimeline("[PHASE2_RESUME_BATCH]",
			fmt.Sprintf("[Phase2] 恢复 %d 个被中断的类别扫描（并行，每类预算 %s）。", len(resumables), auditopts.DefaultCategoryScanTimeout))

		_ = reactloops.DispatchSubAgents(r, task, resumeJobs, reactloops.SubAgentOptions{
			ParentLoop:         loop,
			TimelineMode:       reactloops.SubAgentTimelineFork,
			ExecuteConcurrency: concurrency,
			DefaultJobTimeout:  auditopts.DefaultCategoryScanTimeout,
			ExtraConfigOpts:    auditopts.SubAgentConfigOpts(),
			LoopBuilder: phase2ResumeBatchLoopBuilder{
				state: state, resumeCatalog: resumeCatalog, artifacts: artifacts, scanStates: &scanStates,
			},
		})

		// resume 批结束后逐类复查：仍无 observation → 兜底 auto-finalize。
		for _, p := range resumables {
			category := p.catJob.category
			if countScanObservationsForCategory(state, category.ID) == 0 {
				fallbackFinalizeCategoryScan(r, state, p.scanState, category, nil, artifacts)
			}
			outcomes = append(outcomes, buildCategoryScanOutcome(state, p.catJob, nil))
		}
	}

	// 统一发 outcome 事件（反映 resume 后的真实状态）。
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].index < outcomes[j].index })
	for _, outcome := range outcomes {
		emit.Phase2CategoryOutcome(loop, outcome.index, outcome.total, outcome.category, outcome.findingCount, outcome.incomplete)
		log.Infof("[CodeAudit/Phase2] [%d/%d] Category '%s' complete. Total findings so far: %d",
			outcome.index, outcome.total, outcome.category.ID, len(state.GetFindings()))
	}

	auditDirPath := util.AuditDir(state)
	if err := artifacts.MergeAll(auditDirPath); err != nil {
		log.Warnf("[CodeAudit/Phase2] Failed to merge phase2 artifacts: %v", err)
	}

	return outcomes
}

// prepareCategoryResume 判定一个被中断的类别是否可恢复续扫，并做必要的预处理。
//
// 阶段A中断（有 locked targets 或 discovery 候选）：做 gate+commit 把候选纳入
// 目标并推进到阶段B，返回 true。
// 阶段B中断（已 lock 且仍有未 mark 文件）：清理中断文件的残留 read/grep 计数
// （避免续扫首轮触发 read-repeat/trace-grep guard 误警），返回 true。
// 其余（无 scanState、阶段A空手、阶段B已全部 mark 完）：不可恢复，返回 false。
func prepareCategoryResume(scanState *ScanState, categoryID string) bool {
	if scanState == nil {
		return false
	}
	if shouldResumeCategoryScanFromPhaseA(scanState) {
		autoLocked, skipped := scanState.PrepareDiscoveryGateForPhaseB()
		locked := scanState.CommitToAudit()
		if len(locked) == 0 {
			return false
		}
		log.Warnf("[CodeAudit/Phase2] Category '%s' stuck in phase A; resuming phase B (targets=%d auto_locked=%d skipped=%d)",
			categoryID, len(locked), autoLocked, skipped)
		return true
	}
	if shouldResumeCategoryScanFromPhaseB(scanState) {
		for _, filePath := range scanState.RemainingFiles() {
			scanState.ClearPhaseBReads(filePath)
			scanState.ClearPhaseBGreps(filePath)
		}
		done, total := scanState.Progress()
		log.Warnf("[CodeAudit/Phase2] Category '%s' interrupted in phase B; resuming audit (done=%d total=%d)",
			categoryID, done, total)
		return true
	}
	return false
}

// shouldResumeCategoryScanFromPhaseA returns true when a category sub-agent ended
// stuck in phase A but already has locked targets or fast_context discovery candidates.
func shouldResumeCategoryScanFromPhaseA(scanState *ScanState) bool {
	if scanState == nil || scanState.CurrentPhase() != ScanPhaseSearch {
		return false
	}
	return scanState.TargetFileCount() > 0 || scanState.DiscoveryCandidateCount() > 0
}

// shouldResumeCategoryScanFromPhaseB returns true when a category sub-agent ended
// in phase B with locked targets but not all files marked yet (e.g. cut by the
// per-category wall-clock timeout mid-audit).
func shouldResumeCategoryScanFromPhaseB(scanState *ScanState) bool {
	if scanState == nil || scanState.CurrentPhase() != ScanPhaseAudit {
		return false
	}
	return scanState.TargetFileCount() > 0 && !scanState.AllDone()
}

// buildCategoryScanOutcome 从 state 汇总单个类别的收尾结果。
func buildCategoryScanOutcome(state *model.AuditState, catJob categoryScanJob, execErr error) categoryScanOutcome {
	category := catJob.category
	findingCount := 0
	for _, f := range state.GetFindings() {
		if f.Category == category.ID {
			findingCount++
		}
	}
	return categoryScanOutcome{
		category:     category,
		index:        catJob.index,
		total:        catJob.total,
		incomplete:   countScanObservationsForCategory(state, category.ID) == 0,
		findingCount: findingCount,
		execErr:      execErr,
	}
}

// fallbackFinalizeCategoryScan 是不可恢复类别（或 resume 后仍未完成）的兜底：
// 把剩余目标文件 mark 为 not_vul 并写入 observation，保证 Phase2 结束时每个
// 类别都有 observation 供 Phase3/4 消费（原 finalize.go auto-finalize 的职责
// 移到这里，只在恢复无望时执行，不再销毁可恢复性）。
func fallbackFinalizeCategoryScan(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	scanState *ScanState,
	category model.VulnCategory,
	execErr error,
	artifacts *categoryArtifactStore,
) {
	var remaining []string
	done, total := 0, 0
	if scanState != nil {
		remaining = scanState.RemainingFiles()
		done, total = scanState.Progress()
		for _, filePath := range remaining {
			scanState.MarkFileDoneWithDisposition(filePath, FileDispositionNotVul)
			scanState.ClearPhaseBReads(filePath)
			scanState.ClearPhaseBGreps(filePath)
		}
	}

	stopReason := "auto_finalized_on_timeout"
	obsStatus := model.ScanStatusPartial
	summary := fmt.Sprintf("类别 '%s' 扫描被中断且无法恢复续扫（execErr=%v），已由系统兜底收尾。", category.ID, execErr)
	if scanState == nil || total == 0 {
		// 从未真正扫描（排队即死 / 循环从未构建 scanState）：记录 not_run。
		stopReason = "not_run_interrupted"
		obsStatus = model.ScanStatusNotRun
		summary = fmt.Sprintf("类别 '%s' 扫描未真正开始即被中断（execErr=%v）。", category.ID, execErr)
	} else {
		summary = fmt.Sprintf(
			"类别 '%s' 扫描被中断且无法恢复续扫：%d/%d 文件已审计，剩余 %d 个已由系统兜底标记为 not_vul（execErr=%v）。",
			category.ID, done, total, len(remaining), execErr)
	}

	log.Warnf("[CodeAudit/Phase2] %s", summary)
	r.AddToTimeline("[SCAN_AUTO_FINALIZE]", fmt.Sprintf("[Phase2/%s] %s", category.ID, summary))

	obs := &model.ScanObservation{
		CategoryID:      category.ID,
		CategoryName:    category.Name,
		StopReason:      stopReason,
		CoverageSummary: summary,
		Status:          obsStatus,
		AuditedFiles:    done,
		TargetFiles:     total,
	}
	state.AddScanObservation(obs)

	auditDirPath := util.AuditDir(state)
	if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr != nil {
		log.Warnf("[CodeAudit/Phase2] Failed to create audit dir for fallback finalize: %v", mkErr)
		return
	}
	persistCategoryObservation(artifacts, auditDirPath, category.ID, obs)
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

// phase2CategoryLoopBuilder 是 phase2 fork 子 Agent 的自定义 LoopBuilder，
// 替代原 fork 子 Agent 入口的 loop 构造回调。
type phase2CategoryLoopBuilder struct {
	state      *model.AuditState
	catalog    map[string]categoryScanJob
	artifacts  *categoryArtifactStore
	scanStates *sync.Map
}

func (b phase2CategoryLoopBuilder) Build(prepared *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
	job := prepared.Job
	catJob, ok := b.catalog[job.Identifier]
	if !ok {
		return nil, fmt.Errorf("unknown category job %q", job.Identifier)
	}
	catLoop, scan, err := buildSingleCategoryScanLoop(
		prepared.Invoker, b.state, catJob.category, catJob.index, catJob.total, nil, b.artifacts,
	)
	if scan != nil {
		b.scanStates.Store(catJob.category.ID, scan)
	}
	return catLoop, err
}

// phase2ResumeBatchLoopBuilder 是 phase2 批量恢复续扫子 Agent 的自定义
// LoopBuilder：按 resume job identifier 查回该类别的 scanState，复用既有
// scanState 续跑（阶段A恢复类已在 prepareCategoryResume 做过 gate+commit，
// 阶段B恢复类直接续审剩余文件）。
type phase2ResumeBatchLoopBuilder struct {
	state         *model.AuditState
	resumeCatalog map[string]pendingResume
	artifacts     *categoryArtifactStore
	scanStates    *sync.Map
}

func (b phase2ResumeBatchLoopBuilder) Build(prepared *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
	p, ok := b.resumeCatalog[prepared.Job.Identifier]
	if !ok {
		return nil, fmt.Errorf("unknown category resume job %q", prepared.Job.Identifier)
	}
	catLoop, scan, err := buildSingleCategoryScanLoop(
		prepared.Invoker, b.state, p.catJob.category, p.catJob.index, p.catJob.total, p.scanState, b.artifacts,
	)
	if scan != nil {
		b.scanStates.Store(p.catJob.category.ID, scan)
	}
	return catLoop, err
}
