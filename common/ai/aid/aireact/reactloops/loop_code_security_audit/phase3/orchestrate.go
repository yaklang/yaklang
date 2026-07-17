package phase3

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
		"github.com/yaklang/yaklang/common/log"
)

const DefaultFindingVerifyConcurrency = 5

type findingVerifyJob struct {
	finding *model.Finding
	index   int
	total   int
}

type findingVerifyOutcome struct {
	findingID  string
	index      int
	incomplete bool
	execErr    error
}

// runAllFindingVerifications executes Phase3 finding verification via forked sub-agents.
func runAllFindingVerifications(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
) []findingVerifyOutcome {
	findings := state.GetFindings()
	if len(findings) == 0 {
		return nil
	}

	jobs := make([]reactloops.SubAgentJob, 0, len(findings))
	catalog := make(map[string]findingVerifyJob, len(findings))
	skipped := 0
	for i, finding := range findings {
		if finding == nil || finding.ID == "" {
			continue
		}
		if state.GetVerifiedFindingByID(finding.ID) != nil {
			skipped++
			log.Infof("[CodeAudit/Phase3] Skipping already verified finding %s", finding.ID)
			continue
		}
		goal := fmt.Sprintf("Phase 3 verify: %s — %s", finding.ID, finding.Title)
		jobs = append(jobs, reactloops.SubAgentJob{
			Order:      i + 1,
			Identifier: finding.ID,
			TaskName:   goal,
			Goal:       goal,
		})
		catalog[finding.ID] = findingVerifyJob{
			finding: finding,
			index:   i + 1,
			total:   len(findings),
		}
	}

	if len(jobs) == 0 {
		log.Infof("[CodeAudit/Phase3] All findings already verified or empty (%d skipped), skipping fork", skipped)
		return nil
	}

	concurrency := DefaultFindingVerifyConcurrency
	if len(jobs) < concurrency {
		concurrency = len(jobs)
	}

	log.Infof("[CodeAudit/Phase3] Starting forked sub-agent verify of %d findings (concurrency=%d, skipped=%d)",
		len(jobs), concurrency, skipped)
	r.AddToTimeline("[PHASE3_FORK_START]",
		fmt.Sprintf("Phase 3 fork 子 Agent 并行验证 %d 个 finding（并发 %d，timeline 分支隔离）。", len(jobs), concurrency))

	artifacts := newFindingArtifactStore(state)

	forkResults := reactloops.RunForkJobsConcurrently(
		r, task, jobs, concurrency,
		func(childInvoker aicommon.AIInvokeRuntime, job reactloops.SubAgentJob) (*reactloops.ReActLoop, error) {
			verifyJob, ok := catalog[job.Identifier]
			if !ok {
				return nil, fmt.Errorf("unknown finding job %q", job.Identifier)
			}
			return buildSingleFindingVerifyLoop(
				childInvoker, state, verifyJob.finding, verifyJob.index, verifyJob.total,
			)
		},
	)

	sort.Slice(forkResults, func(i, j int) bool {
		return forkResults[i].Order < forkResults[j].Order
	})

	outcomes := make([]findingVerifyOutcome, 0, len(forkResults))
	for _, forkResult := range forkResults {
		if forkResult == nil {
			continue
		}
		verifyJob, ok := catalog[forkResult.Identifier]
		if !ok {
			continue
		}
		outcome := finalizeFindingVerifyAfterFork(r, loop, state, verifyJob, forkResult)
		outcomes = append(outcomes, outcome)

		if vf := state.GetVerifiedFindingByID(verifyJob.finding.ID); vf != nil {
			verifiedCount := len(state.GetVerifiedVulns())
			emit.Phase3ConcludeFinding(loop, verifyJob.finding.ID, vf.Status, verifiedCount, len(findings), verifyJob.finding.Title)
		}
		log.Infof("[CodeAudit/Phase3] [%d/%d] Finding %s verify done (incomplete=%v)",
			verifyJob.index, verifyJob.total, verifyJob.finding.ID, outcome.incomplete)
	}

	auditDirPath := util.AuditDir(state)
	if err := artifacts.MergeAll(auditDirPath); err != nil {
		log.Warnf("[CodeAudit/Phase3] Failed to merge phase3 artifacts: %v", err)
	}

	return outcomes
}

func finalizeFindingVerifyAfterFork(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	state *model.AuditState,
	job findingVerifyJob,
	forkResult *reactloops.SubAgentResult,
) findingVerifyOutcome {
	finding := job.finding
	if forkResult.ExecErr != nil {
		log.Warnf("[CodeAudit/Phase3] Finding %s forked sub-agent error: %v", finding.ID, forkResult.ExecErr)
	}

	incomplete := state.GetVerifiedFindingByID(finding.ID) == nil
	if incomplete {
		verify := newVerifyState([]*model.Finding{finding})
		FinalizeOnLoopEnd(r, state, verify, false, forkResult.ExecErr)
		if state.GetVerifiedFindingByID(finding.ID) != nil {
			incomplete = false
		}
	}

	if incomplete {
		msg := fmt.Sprintf("Finding %s 验证未调用 conclude_finding 就结束了，已自动标记 uncertain。", finding.ID)
		r.AddToTimeline("[PHASE3_FINDING_INCOMPLETE]", msg)
		log.Warnf("[CodeAudit/Phase3] %s", msg)
	}

	return findingVerifyOutcome{
		findingID:  finding.ID,
		index:      job.index,
		incomplete: incomplete,
		execErr:    forkResult.ExecErr,
	}
}
