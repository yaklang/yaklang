package phase3

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// runRetryVerificationLoop 在 Phase3 第一次批量验证结束后，循环询问用户是否需要
// 重试失败的 finding，直到用户选择不重试或没有可重试项为止。
func runRetryVerificationLoop(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
) {
	for {
		retryFindings := state.GetFindingsNeedingRetry()
		if len(retryFindings) == 0 {
			log.Infof("[CodeAudit/Phase3] No findings need retry, exiting retry loop")
			break
		}

		emit.RetryPrompt(loop, retryFindings, state)

		selectedIDs, shouldStop, err := requestRetrySelection(r, loop, task, retryFindings, state)
		if err != nil {
			log.Warnf("[CodeAudit/Phase3] Retry selection failed: %v", err)
			r.AddToTimeline("[PHASE3_RETRY_SKIP]", fmt.Sprintf("请求重试选择失败：%v，将继续后续阶段。", err))
			break
		}
		if shouldStop || len(selectedIDs) == 0 {
			log.Infof("[CodeAudit/Phase3] User skipped retry or selected nothing")
			r.AddToTimeline("[PHASE3_RETRY_SKIPPED]", "用户选择不重试，继续后续阶段。")
			break
		}

		log.Infof("[CodeAudit/Phase3] User requested retry for findings: %s", strings.Join(selectedIDs, ", "))
		r.AddToTimeline("[PHASE3_RETRY_START]", fmt.Sprintf("开始重试 %d 个 finding：%s", len(selectedIDs), strings.Join(selectedIDs, ", ")))

		// 移除旧结果，允许子 Agent 重新验证这些 finding。
		for _, id := range selectedIDs {
			state.RemoveVerifiedFindingByID(id)
		}

		outcomes := runFindingVerificationsForIDs(r, loop, task, state, selectedIDs)
		mergeRetryOutcomes(r, loop, state, selectedIDs, outcomes)

		state.DedupeVerifiedVulns()

		// 刷新持久化文件，确保前端或后续阶段读取到最新结果。
		if auditDir := util.AuditDir(state); auditDir != "" {
			verifiedFile := fmt.Sprintf("%s%s", auditDir, "/verified_vulns.json")
			if err := state.PersistVerifiedVulns(verifiedFile); err != nil {
				log.Warnf("[CodeAudit/Phase3] Failed to persist verified_vulns after retry: %v", err)
			}
		}

		emit.VerifyComplete(loop, "重试后", state.GetStats())
	}
}

// requestRetrySelection 通过 AskForClarification 向前端展示可重试 finding 列表，
// 并返回用户选中的 finding ID 列表。
func requestRetrySelection(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	retryFindings []*model.Finding,
	state *model.AuditState,
) ([]string, bool, error) {
	invoker := loop.GetInvoker()
	if invoker == nil {
		return nil, true, utils.Error("loop invoker is nil")
	}

	ctx := invoker.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}

	retryItems := make([]map[string]any, 0, len(retryFindings))
	findingIDs := make([]string, 0, len(retryFindings))
	for _, f := range retryFindings {
		if f == nil || f.ID == "" {
			continue
		}
		retryCount, lastError := state.GetVerifiedFindingRetryInfo(f.ID)
		item := map[string]any{
			"id":          f.ID,
			"title":       f.Title,
			"category":    f.Category,
			"severity":    f.Severity,
			"confidence":  f.Confidence,
			"retry_count": retryCount,
			"last_error":  lastError,
		}
		retryItems = append(retryItems, item)
		findingIDs = append(findingIDs, f.ID)
	}

	if len(retryItems) == 0 {
		return nil, true, nil
	}

	payload := map[string]any{
		"finding_ids": findingIDs,
		"findings":    retryItems,
	}
	payloadJSON, _ := json.Marshal(payload)

	question := fmt.Sprintf(
		"Phase 3 验证完成，以下 %d 个 finding 验证失败或未完成，请选择要重试的项（输入 ID 列表，逗号分隔；输入空或 no 表示不重试）：\n%s",
		len(retryItems), formatRetryOptionsText(retryItems),
	)

	suggestion := invoker.AskForClarification(ctx, question, []string{string(payloadJSON)})
	if suggestion == "" {
		return nil, true, nil
	}

	clean := strings.ToLower(strings.TrimSpace(suggestion))
	if clean == "" || clean == "no" || clean == "none" || clean == "skip" || clean == "n" {
		return nil, true, nil
	}

	selected := parseRetrySelection(suggestion, findingIDs)
	if len(selected) == 0 {
		return nil, true, nil
	}
	return selected, false, nil
}

// formatRetryOptionsText 把可重试 finding 渲染成人类可读列表，用于展示在 timeline 中。
func formatRetryOptionsText(items []map[string]any) string {
	var b strings.Builder
	for _, item := range items {
		id := utils.InterfaceToString(item["id"])
		title := utils.InterfaceToString(item["title"])
		category := utils.InterfaceToString(item["category"])
		retryCount := utils.InterfaceToInt(item["retry_count"])
		b.WriteString(fmt.Sprintf("  • %s [%s] %s", id, category, title))
		if retryCount > 0 {
			b.WriteString(fmt.Sprintf(" (已重试 %d 次)", retryCount))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseRetrySelection 解析用户返回的字符串，提取有效的 finding ID。
func parseRetrySelection(input string, validIDs []string) []string {
	validSet := make(map[string]struct{}, len(validIDs))
	for _, id := range validIDs {
		validSet[id] = struct{}{}
	}

	var selected []string
	seen := make(map[string]struct{})
	for _, raw := range strings.Split(input, ",") {
		id := strings.ToUpper(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := validSet[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, id)
	}
	return selected
}

// runFindingVerificationsForIDs 为选中的 finding ID 重新 fork 子 Agent 验证。
func runFindingVerificationsForIDs(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	state *model.AuditState,
	ids []string,
) []findingVerifyOutcome {
	if len(ids) == 0 {
		return nil
	}

	findings := state.GetFindings()
	idSet := make(map[string]*model.Finding, len(findings))
	for _, f := range findings {
		if f == nil || f.ID == "" {
			continue
		}
		idSet[f.ID] = f
	}

	sorted := sortFindingsByConfidenceDesc(func() []*model.Finding {
		var out []*model.Finding
		for _, id := range ids {
			if f, ok := idSet[id]; ok {
				out = append(out, f)
			}
		}
		return out
	}())

	jobs := make([]reactloops.SubAgentJob, 0, len(sorted))
	catalog := make(map[string]findingVerifyJob, len(sorted))
	for i, finding := range sorted {
		if finding == nil || finding.ID == "" {
			continue
		}
		goal := fmt.Sprintf("Phase 3 retry verify: %s — %s", finding.ID, finding.Title)
		jobs = append(jobs, reactloops.SubAgentJob{
			Order:      i + 1,
			Identifier: finding.ID,
			TaskName:   goal,
			Goal:       goal,
		})
		catalog[finding.ID] = findingVerifyJob{
			finding: finding,
			index:   i + 1,
			total:   len(sorted),
		}
	}

	if len(jobs) == 0 {
		return nil
	}

	concurrency := reactloops.ResolveSubAgentConcurrency(loop.GetMaxSubAgents(), len(jobs))
	log.Infof("[CodeAudit/Phase3] Starting retry verify of %d findings (concurrency=%d)", len(jobs), concurrency)
	r.AddToTimeline("[PHASE3_RETRY_FORK]", fmt.Sprintf("重试 fork %d 个 finding（并发=%d）。", len(jobs), concurrency))

	forkResults := reactloops.DispatchSubAgents(r, task, jobs, reactloops.SubAgentOptions{
		ParentLoop:         loop,
		TimelineMode:       reactloops.SubAgentTimelineFork,
		ExecuteConcurrency: concurrency,
		LoopBuilder:        phase3FindingLoopBuilder{state: state, catalog: catalog},
	})

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
			verifiedCount := state.GetVerifiedVulnCount()
			emit.Phase3ConcludeFinding(loop, verifyJob.finding.ID, vf.Status, verifiedCount, len(ids), verifyJob.finding.Title)
		}
		log.Infof("[CodeAudit/Phase3] Retry [%d/%d] Finding %s verify done (incomplete=%v)",
			verifyJob.index, verifyJob.total, verifyJob.finding.ID, outcome.incomplete)
	}

	return outcomes
}

// mergeRetryOutcomes 把重试结果合并到 AuditState，更新重试次数与最近错误。
func mergeRetryOutcomes(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	state *model.AuditState,
	requestedIDs []string,
	outcomes []findingVerifyOutcome,
) {
	for _, id := range requestedIDs {
		vf := state.GetVerifiedFindingByID(id)
		if vf == nil {
			// 子 Agent 仍未产出结论，FinalizeOnLoopEnd 已自动标 uncertain。
			vf = state.GetVerifiedFindingByID(id)
		}
		if vf == nil {
			continue
		}
		vf.RetryCount++
		state.UpsertVerifiedFinding(vf)

		loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_verify_finding", map[string]any{
			"finding_id":  id,
			"status":      string(vf.Status),
			"confidence":  vf.Confidence,
			"reason":      vf.Reason,
			"retry_count": vf.RetryCount,
		})
	}

	for _, outcome := range outcomes {
		_ = outcome
	}

	r.AddToTimeline("[PHASE3_RETRY_DONE]", fmt.Sprintf("重试完成：%d 个 finding 已更新验证结果。", len(requestedIDs)))
}
