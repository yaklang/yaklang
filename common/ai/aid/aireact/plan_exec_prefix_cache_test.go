package aireact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestPlanExec_PrefixCacheStableWithMockedTieredAI(t *testing.T) {
	const (
		intelligentModel = "mock-intelligent-planexec"
		lightweightModel = "mock-lightweight-planexec"
		toolName         = "mock_plan_exec_prefix_cache_tool"
	)

	originalTiered := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(nil)
	t.Cleanup(func() {
		consts.SetTieredAIConfig(originalTiered)
	})

	stages := []planExecMockStage{
		{
			Name:       "整理输入上下文",
			Identifier: "inventory_inputs",
			Goal:       "调用 mock_plan_exec_prefix_cache_tool 为 inventory_inputs 生成固定的输入目录快照，确认执行边界与约束信息。",
			ToolParam:  "inventory_inputs",
			Stdout: buildPlanExecMockToolStdout(
				"inventory_inputs",
				"captured deterministic input snapshot",
				[]string{
					"workspace tree normalized for deterministic replay",
					"input boundaries locked for prefix cache inspection",
				},
				[]string{
					"artifact://inventory_inputs/input_snapshot.json",
					"artifact://inventory_inputs/constraints.md",
				},
			),
			Result: buildPlanExecMockToolResult(
				"inventory_inputs",
				"captured deterministic input snapshot",
				[]string{
					"pure-mock-ai",
					"no-network",
					"timeline-regression-fixture",
				},
				[]string{
					"workspace tree normalized for deterministic replay",
					"input boundaries locked for prefix cache inspection",
				},
				[]string{
					"artifact://inventory_inputs/input_snapshot.json",
					"artifact://inventory_inputs/constraints.md",
				},
				"",
			),
		},
		{
			Name:       "提取稳定执行证据",
			Identifier: "collect_evidence",
			Goal:       "调用 mock_plan_exec_prefix_cache_tool 为 collect_evidence 产出固定证据，沉淀可复查的执行结论。",
			ToolParam:  "collect_evidence",
			Stdout: buildPlanExecMockToolStdout(
				"collect_evidence",
				"collected deterministic evidence pack",
				[]string{
					"tool output envelope kept stable across deterministic replay",
					"timeline shared prefix preserved in evidence packaging",
				},
				[]string{
					"artifact://collect_evidence/evidence_bundle.json",
					"artifact://collect_evidence/replay_notes.md",
				},
			),
			Result: buildPlanExecMockToolResult(
				"collect_evidence",
				"collected deterministic evidence pack",
				[]string{
					"tool-output-stable",
					"timeline-shared-prefix",
					"prompt-prefix-cache-regression",
				},
				[]string{
					"tool output envelope kept stable across deterministic replay",
					"timeline shared prefix preserved in evidence packaging",
				},
				[]string{
					"artifact://collect_evidence/evidence_bundle.json",
					"artifact://collect_evidence/replay_notes.md",
				},
				"",
			),
		},
		{
			Name:       "交叉核对结果",
			Identifier: "cross_check_results",
			Goal:       "调用 mock_plan_exec_prefix_cache_tool 为 cross_check_results 执行固定核对，确认前序信息相互一致。",
			ToolParam:  "cross_check_results",
			Stdout: buildPlanExecMockToolStdout(
				"cross_check_results",
				"cross-check completed deterministically",
				[]string{
					"shared artifacts aligned with previously captured evidence",
					"no unexpected drift detected in deterministic replay output",
				},
				[]string{
					"artifact://cross_check_results/alignment_report.json",
					"artifact://cross_check_results/mismatch_index.txt",
				},
			),
			Result: buildPlanExecMockToolResult(
				"cross_check_results",
				"cross-check completed deterministically",
				[]string{
					"mismatches=0",
					"alignment=stable",
					"timeline-prefix-reuse=confirmed",
				},
				[]string{
					"shared artifacts aligned with previously captured evidence",
					"no unexpected drift detected in deterministic replay output",
				},
				[]string{
					"artifact://cross_check_results/alignment_report.json",
					"artifact://cross_check_results/mismatch_index.txt",
				},
				"",
			),
		},
		{
			Name:       "汇总交付结论",
			Identifier: "finalize_delivery",
			Goal:       "调用 mock_plan_exec_prefix_cache_tool 为 finalize_delivery 生成最终交付摘要输入，准备完成整个任务。",
			ToolParam:  "finalize_delivery",
			Stdout: buildPlanExecMockToolStdout(
				"finalize_delivery",
				"prepared final delivery package",
				[]string{
					"delivery payload assembled from deterministic intermediate artifacts",
					"final note ready for prefix cache regression handoff",
				},
				[]string{
					"artifact://finalize_delivery/delivery_package.md",
					"artifact://finalize_delivery/execution_note.txt",
				},
			),
			Result: buildPlanExecMockToolResult(
				"finalize_delivery",
				"prepared final delivery package",
				[]string{
					"handoff-ready",
					"deterministic-mock-output",
					"prefix-cache-guardrail-preserved",
				},
				[]string{
					"delivery payload assembled from deterministic intermediate artifacts",
					"final note ready for prefix cache regression handoff",
				},
				[]string{
					"artifact://finalize_delivery/delivery_package.md",
					"artifact://finalize_delivery/execution_note.txt",
				},
				"mock final execution note",
			),
		},
	}

	stageByToolParam := make(map[string]planExecMockStage, len(stages))
	for _, stage := range stages {
		stageByToolParam[stage.ToolParam] = stage
	}

	probe := newPlanExecPromptProbe()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 256)

	var toolCallsMu sync.Mutex
	var toolCalls []string
	var stageCursorMu sync.Mutex
	decisionStageIdx := 0
	toolParamStageIdx := 0
	progressStageIdx := 0

	mockTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("subtask_id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			subtaskID := params.GetString("subtask_id")
			stage, ok := stageByToolParam[subtaskID]
			if !ok {
				return nil, utils.Errorf("unexpected subtask_id: %s", subtaskID)
			}
			toolCallsMu.Lock()
			toolCalls = append(toolCalls, subtaskID)
			toolCallsMu.Unlock()
			_, _ = io.WriteString(stdout, stage.Stdout)
			return stage.Result, nil
		}),
	)
	require.NoError(t, err)

	intelligentMock := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		probe.Observe(prompt)

		switch {
		case isPlanExecPlanExplorationPrompt(prompt):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":                "finish_exploration",
				"human_readable_thought": "已收集足够事实，开始生成指导文档和执行计划",
			})), nil

		case isPlanExecGuidanceDocPrompt(prompt):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action": "plan_guidance_document",
				"document": strings.Join([]string{
					"# 目标定义",
					"- 以纯 mock 方式完成 PlanAndExec 长链路验证。",
					"# 执行路径（方法论视角）",
					"- 先固定规划，再按子任务顺序执行 deterministic mock tool。",
					"# 验收标准",
					"- 所有子任务都要落下可复查的固定结果。",
					"# 动态重规划与纠错纠偏",
					"- 若 mock 输出不匹配预期，立即停止并报告。",
				}, "\n"),
			})), nil

		case isPlanExecPlanFromDocPrompt(prompt):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":              "plan_from_document",
				"main_task":            "验证纯 mock 的 PlanAndExec prefix cache 稳定性",
				"main_task_identifier": "verify_plan_exec_prefix_cache",
				"main_task_goal":       "以纯 mock 的方式走完整条 PlanAndExec 链路，并保证主模型 prompt 前缀稳定。",
				"tasks": []map[string]any{
					{
						"subtask_name":       stages[0].Name,
						"subtask_identifier": stages[0].Identifier,
						"subtask_goal":       stages[0].Goal,
						"depends_on":         []string{},
					},
					{
						"subtask_name":       stages[1].Name,
						"subtask_identifier": stages[1].Identifier,
						"subtask_goal":       stages[1].Goal,
						"depends_on":         []string{stages[0].Name},
					},
					{
						"subtask_name":       stages[2].Name,
						"subtask_identifier": stages[2].Identifier,
						"subtask_goal":       stages[2].Goal,
						"depends_on":         []string{stages[1].Name},
					},
					{
						"subtask_name":       stages[3].Name,
						"subtask_identifier": stages[3].Identifier,
						"subtask_goal":       stages[3].Goal,
						"depends_on":         []string{stages[2].Name},
					},
				},
			})), nil

		case utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") &&
			!strings.Contains(prompt, "PROGRESS_TASK_"):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action": "object",
				"next_action": map[string]any{
					"type":                 "request_plan_and_execution",
					"plan_request_payload": "为纯 mock prefix cache 回归测试执行稳定的 plan and execute 长链路",
				},
				"human_readable_thought": "复杂任务需要进入 plan and execute",
				"cumulative_summary":     "delegate to mocked plan and execution",
			})), nil

		case isToolParamGenerationPrompt(prompt, toolName):
			stageCursorMu.Lock()
			if toolParamStageIdx >= len(stages) {
				stageCursorMu.Unlock()
				return nil, utils.Errorf("unexpected intelligent tool-param prompt overflow: %s", utils.ShrinkString(prompt, 240))
			}
			stage := stages[toolParamStageIdx]
			toolParamStageIdx++
			stageCursorMu.Unlock()
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action": "call-tool",
				"tool":    toolName,
				"params": map[string]any{
					"subtask_id": stage.ToolParam,
				},
			})), nil

		case isVerifySatisfactionPrompt(prompt):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":        "verify-satisfaction",
				"user_satisfied": true,
				"reasoning":      "all deterministic mock subtasks finished successfully",
			})), nil

		case utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_", "directly_answer", "require_tool"):
			stageCursorMu.Lock()
			if decisionStageIdx >= len(stages) {
				stageCursorMu.Unlock()
				return nil, utils.Errorf("unexpected intelligent subtask decision prompt overflow: %s", utils.ShrinkString(prompt, 240))
			}
			stage := stages[decisionStageIdx]
			decisionStageIdx++
			stageCursorMu.Unlock()
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action": "object",
				"next_action": map[string]any{
					"type":                 "require_tool",
					"tool_require_payload": toolName,
				},
				"human_readable_thought": fmt.Sprintf("执行子任务 %s，需要调用 mock tool", stage.Identifier),
				"cumulative_summary":     fmt.Sprintf("%s ready for deterministic tool execution", stage.Identifier),
			})), nil

		case utils.MatchAllOfSubString(prompt, "continue-current-task", "proceed-next-task", "task-failed"):
			stageCursorMu.Lock()
			if progressStageIdx >= len(stages) {
				stageCursorMu.Unlock()
				return nil, utils.Errorf("unexpected intelligent task-progress prompt overflow: %s", utils.ShrinkString(prompt, 240))
			}
			stage := stages[progressStageIdx]
			progressStageIdx++
			stageCursorMu.Unlock()
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":            "proceed-next-task",
				"status_summary":     fmt.Sprintf("%s 已生成稳定结果", stage.Identifier),
				"task_short_summary": fmt.Sprintf("%s done", stage.Identifier),
			})), nil

		case utils.MatchAllOfSubString(prompt, "任务执行引擎", "task_long_summary") &&
			!strings.Contains(prompt, "PROGRESS_TASK_"):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":            "summary",
				"status_summary":     "all mocked subtasks completed",
				"task_short_summary": "completed",
				"task_long_summary":  "all deterministic mocked subtasks finished and the prefix cache regression guardrail stayed stable",
			})), nil

		case isDirectAnswerPrompt(prompt):
			return newMockAIResponse(i, intelligentModel, mustJSONString(map[string]any{
				"@action":        "directly_answer",
				"answer_payload": "mocked plan and execution completed successfully",
			})), nil
		}

		return nil, utils.Errorf("unexpected intelligent prompt: %s", utils.ShrinkString(prompt, 240))
	}

	lightweightMock := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()

		switch {
		case isPlanExecFactsHookPrompt(prompt):
			return newMockAIResponse(i, lightweightModel, mustJSONString(map[string]any{
				"@action": "plan_facts_hook",
				"facts": strings.Join([]string{
					"- 当前链路使用纯 mock AI callback",
					"- 所有工具结果均由 deterministic mock tool 产生",
				}, "\n"),
			})), nil
		}

		return nil, utils.Errorf("unexpected lightweight prompt: %s", utils.ShrinkString(prompt, 240))
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(intelligentMock),
		aicommon.WithQualityPriorityAICallback(intelligentMock),
		aicommon.WithSpeedPriorityAICallback(lightweightMock),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithTools(mockTool),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "请用纯 mock 方式执行 PlanAndExec prefix cache 回归链路",
	}

	var (
		sawPlanStart      bool
		sawPlanEnd        bool
		reactTaskComplete bool
		summaryModels     []string
	)

	timeout := time.After(30 * time.Second)
LOOP:
	for {
		select {
		case e := <-out:
			switch e.Type {
			case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
				sawPlanStart = true
			case schema.EVENT_TYPE_END_PLAN_AND_EXECUTION:
				sawPlanEnd = true
			case schema.EVENT_TYPE_AI_CALL_SUMMARY:
				var payload map[string]any
				if err := json.Unmarshal(e.Content, &payload); err == nil {
					summaryModels = append(summaryModels, utils.InterfaceToString(payload["model_name"]))
				}
			case schema.EVENT_TYPE_STRUCTURED:
				if e.NodeId != "react_task_status_changed" {
					continue
				}
				var payload map[string]any
				require.NoError(t, json.Unmarshal(e.Content, &payload))
				if utils.InterfaceToString(payload["react_task_now_status"]) == "completed" {
					reactTaskComplete = true
				}
			}
			toolCallsMu.Lock()
			toolCallCount := len(toolCalls)
			toolCallsMu.Unlock()
			if sawPlanStart && sawPlanEnd && reactTaskComplete && toolCallCount == len(stages) {
				break LOOP
			}
		case <-timeout:
			break LOOP
		}
	}

	records := probe.Records()
	diagnostics := formatPlanExecProbeDiagnostics(records)

	require.Truef(t, sawPlanStart, "expected EVENT_TYPE_START_PLAN_AND_EXECUTION\nmodels=%v\n%s", summaryModels, diagnostics)
	require.Truef(t, sawPlanEnd, "expected EVENT_TYPE_END_PLAN_AND_EXECUTION\nmodels=%v\n%s", summaryModels, diagnostics)
	require.Truef(t, reactTaskComplete, "expected react_task_status_changed=completed\nmodels=%v\n%s", summaryModels, diagnostics)

	toolCallsMu.Lock()
	gotToolCalls := append([]string(nil), toolCalls...)
	toolCallsMu.Unlock()
	require.Equal(t,
		[]string{
			stages[0].ToolParam,
			stages[1].ToolParam,
			stages[2].ToolParam,
			stages[3].ToolParam,
		},
		gotToolCalls,
		"expected each mocked subtask to execute the deterministic tool exactly once",
	)

	require.GreaterOrEqualf(t, len(records), 5, "expected at least 5 intelligent prompts\n%s", diagnostics)

	for _, rec := range records {
		require.NotContainsf(t, rec.Sections, aicache.SectionRaw, "intelligent prompt must not contain raw section\n%s", formatPlanExecProbeRecord(rec))
		require.Containsf(t, rec.Sections, aicache.SectionHighStatic, "intelligent prompt must contain high-static\n%s", formatPlanExecProbeRecord(rec))
		require.Containsf(t, rec.Sections, aicache.SectionDynamic, "intelligent prompt must contain dynamic\n%s", formatPlanExecProbeRecord(rec))
		require.NotEmptyf(t, rec.HighStaticHash, "intelligent prompt must expose a high-static hash\n%s", formatPlanExecProbeRecord(rec))
	}

	var totalPromptTokens int
	var totalHitPrefixTokens int
	var promptsWithPrefixHits int
	highStaticHashes := make(map[string]struct{})
	for _, rec := range records {
		totalPromptTokens += rec.TotalPromptTokens
		totalHitPrefixTokens += rec.HitPrefixTokens
		if rec.HitPrefixTokens > 0 {
			promptsWithPrefixHits++
		}
		highStaticHashes[rec.HighStaticHash] = struct{}{}
	}
	require.Positivef(t, totalPromptTokens, "expected positive total prompt tokens\n%s", diagnostics)
	globalHitTokenRatio := planExecRatio(totalHitPrefixTokens, totalPromptTokens)

	require.GreaterOrEqualf(t, globalHitTokenRatio, 0.30, "global hit token ratio below threshold: %.4f\n%s", globalHitTokenRatio, diagnostics)
	require.Positivef(t, promptsWithPrefixHits, "expected at least one intelligent prompt with prefix hit\n%s", diagnostics)
	require.NotEmptyf(t, highStaticHashes, "expected at least one intelligent high-static hash\n%s", diagnostics)

	t.Logf("intelligent prompts=%d", len(records))
	t.Logf("total prompt tokens=%d", totalPromptTokens)
	t.Logf("total hit prefix tokens=%d", totalHitPrefixTokens)
	t.Logf("global hit token ratio=%.4f", globalHitTokenRatio)
	t.Logf("intelligent prompts with prefix hits=%d", promptsWithPrefixHits)
	t.Logf("distinct intelligent high-static hashes=%d", len(highStaticHashes))
}

type planExecMockStage struct {
	Name       string
	Identifier string
	Goal       string
	ToolParam  string
	Result     map[string]any
	Stdout     string
}

type planExecPromptProbe struct {
	mu      sync.Mutex
	history []*planExecPromptHistory
	records []*planExecPromptRecord
}

type planExecPromptHistory struct {
	hashes   []string
	contents []string
}

type planExecPromptRecord struct {
	Seq               int
	TotalPromptTokens int
	HitPrefixTokens   int
	TokenHitRatio     float64
	PrefixHitChunks   int
	Sections          []string
	HighStaticHash    string
}

func newPlanExecPromptProbe() *planExecPromptProbe {
	return &planExecPromptProbe{}
}

func (p *planExecPromptProbe) Observe(prompt string) *planExecPromptRecord {
	split := aicache.Split(prompt)
	hashes := make([]string, 0, len(split.Chunks))
	contents := make([]string, 0, len(split.Chunks))
	sections := make([]string, 0, len(split.Chunks))
	seenSections := make(map[string]struct{}, len(split.Chunks))

	totalPromptTokens := ytoken.CalcTokenCount(prompt)
	highStaticHash := ""
	for _, chunk := range split.Chunks {
		if chunk == nil {
			continue
		}
		hashes = append(hashes, chunk.Hash)
		contents = append(contents, chunk.Content)
		if _, ok := seenSections[chunk.Section]; !ok {
			sections = append(sections, chunk.Section)
			seenSections[chunk.Section] = struct{}{}
		}
		if chunk.Section == aicache.SectionHighStatic && highStaticHash == "" {
			highStaticHash = chunk.Hash
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bestLCP := 0
	bestPrefixBytes := 0
	bestPrefixContent := ""
	for _, prev := range p.history {
		if prev == nil {
			continue
		}
		lcp := planExecCommonPrefixLen(hashes, prev.hashes)
		prefixContent, prefixBytes := planExecBuildPrefixContent(contents, prev.contents, lcp)
		if prefixBytes > bestPrefixBytes || (prefixBytes == bestPrefixBytes && lcp > bestLCP) {
			bestLCP = lcp
			bestPrefixBytes = prefixBytes
			bestPrefixContent = prefixContent
		}
	}

	hitPrefixTokens := ytoken.CalcTokenCount(bestPrefixContent)

	record := &planExecPromptRecord{
		Seq:               len(p.records) + 1,
		TotalPromptTokens: totalPromptTokens,
		HitPrefixTokens:   hitPrefixTokens,
		TokenHitRatio:     planExecRatio(hitPrefixTokens, totalPromptTokens),
		PrefixHitChunks:   bestLCP,
		Sections:          sections,
		HighStaticHash:    highStaticHash,
	}

	p.records = append(p.records, record)
	p.history = append(p.history, &planExecPromptHistory{
		hashes:   hashes,
		contents: contents,
	})

	return record
}

func (p *planExecPromptProbe) Records() []*planExecPromptRecord {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*planExecPromptRecord, 0, len(p.records))
	for _, rec := range p.records {
		cp := *rec
		cp.Sections = append([]string(nil), rec.Sections...)
		out = append(out, &cp)
	}
	return out
}

func planExecCommonPrefixLen(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func planExecBuildPrefixContent(current, previous []string, matchedChunks int) (string, int) {
	var prefix strings.Builder
	prefixBytes := 0

	for i := 0; i < matchedChunks && i < len(current); i++ {
		prefix.WriteString(current[i])
		prefixBytes += len(current[i])
	}

	if matchedChunks < len(current) && matchedChunks < len(previous) {
		partialBytes := planExecStringCommonPrefixLen(current[matchedChunks], previous[matchedChunks])
		if partialBytes > 0 {
			partial := planExecTrimToValidUTF8Prefix(current[matchedChunks][:partialBytes])
			prefix.WriteString(partial)
			prefixBytes += len(partial)
		}
	}

	return prefix.String(), prefixBytes
}

func planExecStringCommonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func planExecTrimToValidUTF8Prefix(s string) string {
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func planExecRatio(hit, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func formatPlanExecProbeDiagnostics(records []*planExecPromptRecord) string {
	if len(records) == 0 {
		return "no intelligent prompts observed"
	}
	totalPromptTokens, totalHitPrefixTokens := summarizePlanExecProbe(records)
	lines := make([]string, 0, len(records)+1)
	lines = append(lines, fmt.Sprintf(
		"total_prompt_tokens=%d total_hit_prefix_tokens=%d global_hit_token_ratio=%.4f",
		totalPromptTokens,
		totalHitPrefixTokens,
		planExecRatio(totalHitPrefixTokens, totalPromptTokens),
	))
	for _, rec := range records {
		lines = append(lines, formatPlanExecProbeRecord(rec))
	}
	return strings.Join(lines, "\n")
}

func formatPlanExecProbeRecord(rec *planExecPromptRecord) string {
	if rec == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"seq=%d total_tokens=%d hit_prefix_tokens=%d token_hit_ratio=%.4f prefix_hit_chunks=%d sections=%s high_static=%s",
		rec.Seq,
		rec.TotalPromptTokens,
		rec.HitPrefixTokens,
		rec.TokenHitRatio,
		rec.PrefixHitChunks,
		strings.Join(rec.Sections, ","),
		shortPlanExecHash(rec.HighStaticHash),
	)
}

func summarizePlanExecProbe(records []*planExecPromptRecord) (int, int) {
	var totalPromptTokens int
	var totalHitPrefixTokens int
	for _, rec := range records {
		if rec == nil {
			continue
		}
		totalPromptTokens += rec.TotalPromptTokens
		totalHitPrefixTokens += rec.HitPrefixTokens
	}
	return totalPromptTokens, totalHitPrefixTokens
}

func shortPlanExecHash(hash string) string {
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

func buildPlanExecMockToolStdout(stageID, summary string, checkpoints, artifacts []string) string {
	lines := []string{
		"mock-tool-stage-report",
		"stage_id: " + stageID,
		"summary: " + summary,
		"shared_prefix: timeline-fixture: deterministic-prefix-cache-regression",
	}
	for i, checkpoint := range checkpoints {
		lines = append(lines, fmt.Sprintf("checkpoint[%d]: %s", i+1, checkpoint))
	}
	for i, artifact := range artifacts {
		lines = append(lines, fmt.Sprintf("artifact[%d]: %s", i+1, artifact))
	}
	base := strings.Join(lines, "\n")

	const targetStdoutBytes = 1024
	if len(base) >= targetStdoutBytes {
		return base
	}

	var stdout strings.Builder
	stdout.WriteString(base)
	fillerTemplate := "payload_fill[%02d]: deterministic-prefix-cache-regression timeline-fixture block=abcdefghijklmnopqrstuvwxyz0123456789 repeated-content-for-mock-stdout-padding"
	for i := 1; stdout.Len() < targetStdoutBytes; i++ {
		stdout.WriteString("\n")
		stdout.WriteString(fmt.Sprintf(fillerTemplate, i))
	}
	return stdout.String()
}

func buildPlanExecMockToolResult(stageID, summary string, evidence, checkpoints, artifacts []string, deliverable string) map[string]any {
	result := map[string]any{
		"stage":         stageID,
		"summary":       summary,
		"shared_prefix": "timeline-fixture: deterministic-prefix-cache-regression",
		"evidence":      evidence,
		"checkpoints":   checkpoints,
		"timeline_note": strings.Join([]string{
			"Mock tool result intentionally returns richer deterministic material.",
			"These fields are used to exercise larger timeline payloads without changing AI routing behavior.",
		}, "\n"),
		"metrics": map[string]any{
			"deterministic_replay":         true,
			"expected_timeline_artifacts":  len(artifacts),
			"prefix_cache_guardrail":       "global_hit_token_ratio>=0.30",
			"timeline_payload_profile":     "expanded-mock-result",
			"tool_output_contract_version": "v2",
		},
	}

	artifactRecords := make([]map[string]any, 0, len(artifacts))
	for i, artifact := range artifacts {
		artifactRecords = append(artifactRecords, map[string]any{
			"index":   i + 1,
			"path":    artifact,
			"kind":    "mock-artifact",
			"status":  "stable",
			"profile": "timeline-regression-fixture",
		})
	}
	result["artifacts"] = artifactRecords

	if deliverable != "" {
		result["deliverable"] = deliverable
	}
	return result
}

func newMockAIResponse(i aicommon.AICallerConfigIf, modelName string, payload string) *aicommon.AIResponse {
	rsp := i.NewAIResponse()
	rsp.SetModelInfo("mock-provider", modelName)
	rsp.EmitOutputStream(bytes.NewBufferString(payload))
	rsp.Close()
	return rsp
}

func mustJSONString(v any) string {
	return string(utils.Jsonify(v))
}

func matchPlanExecStage(prompt string, stages []planExecMockStage) (planExecMockStage, bool) {
	searchScopes := []string{
		extractLastCurrentTaskBlock(prompt),
		prompt,
	}

	for _, scope := range searchScopes {
		if scope == "" {
			continue
		}
		for _, stage := range stages {
			if strings.Contains(scope, "任务名称: "+stage.Name) ||
				strings.Contains(scope, "任务目标: "+stage.Goal) ||
				strings.Contains(scope, stage.Identifier) ||
				strings.Contains(scope, stage.Name) ||
				strings.Contains(scope, stage.ToolParam) {
				return stage, true
			}
		}
	}
	return planExecMockStage{}, false
}

func extractLastCurrentTaskBlock(prompt string) string {
	bestStart := -1
	bestBlock := ""
	for _, pair := range [][2]string{
		{"--- CURRENT_TASK ---", "--- CURRENT_TASK_END ---"},
		{"<|CURRENT_TASK_", "<|CURRENT_TASK_END_"},
	} {
		start := strings.LastIndex(prompt, pair[0])
		if start < 0 {
			continue
		}

		rest := prompt[start:]
		end := strings.Index(rest, pair[1])
		if end < 0 {
			if start > bestStart {
				bestStart = start
				bestBlock = rest
			}
			continue
		}
		if start > bestStart {
			bestStart = start
			bestBlock = rest[:end]
		}
	}
	return bestBlock
}

func isPlanExecPlanExplorationPrompt(prompt string) bool {
	return strings.Contains(prompt, "任务规划使命") && strings.Contains(prompt, "finish_exploration")
}

func isPlanExecFactsHookPrompt(prompt string) bool {
	return strings.Contains(prompt, `"const": "plan_facts_hook"`) ||
		(strings.Contains(prompt, "plan_facts_hook") && strings.Contains(prompt, `"facts"`))
}

func isPlanExecGuidanceDocPrompt(prompt string) bool {
	return strings.Contains(prompt, `"const": "plan_guidance_document"`)
}

func isPlanExecPlanFromDocPrompt(prompt string) bool {
	return strings.Contains(prompt, `"const": "plan_from_document"`)
}
