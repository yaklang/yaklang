package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// This exercises the public AI engine and native runtime. Only
// the provider is scripted: no fake coordinator/worker or fake event bridge.
func TestCoordinatorAIMNativeIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	workdir := t.TempDir()
	sourceFile := filepath.Join(workdir, "source.txt")
	require.NoError(t, os.WriteFile(sourceFile, []byte("coordinator-exploration-sentinel: verified source"), 0600))
	var mu sync.Mutex
	var eventsMu sync.Mutex
	var events []*schema.AiOutputEvent
	var approvalErr error
	var workerCalls, coordinatorCalls, modelReviews int
	workerStep := make(map[string]int)
	saved := false
	reported := false
	var prompts []string
	explored := false
	taskPattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+)`)
	model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		prompt := req.GetPrompt()
		prompts = append(prompts, prompt)
		if strings.Contains(prompt, "执行已批准的冻结任务书。") {
			workerCalls++
			index := req.GetTaskIndex()
			if index == "" {
				index = "single"
			}
			step := workerStep[index]
			workerStep[index] = step + 1
			if index == "1-2" && step == 0 {
				require.Contains(t, prompt, "smoke.task.1-1", "dependent worker must see the first task's session evidence")
			}
			switch step {
			case 0:
				return nativeResponse(c, req, "save_evidence", map[string]any{"evidence_id": "smoke.task." + index, "evidence_content": "Task " + index + " independently confirmed its assigned test result."})
			case 1:
				return nativeResponse(c, req, "submit_task_result", map[string]any{"summary": "Task " + index + " result verified", "evidence_ids": []string{"smoke.task." + index}})
			default:
				return nativeResponse(c, req, "finish", map[string]any{})
			}
		}
		coordinatorCalls++
		if !explored {
			explored = true
			return nativeResponse(c, req, "directly_call_tool", map[string]any{"directly_call_tool_name": "read_file", "directly_call_tool_params": map[string]any{"file": sourceFile}, "directly_call_reason": "Read the source before preparing the plan."})
		}
		if strings.Contains(prompt, "已有计划：false") {
			return nativeResponse(c, req, "create_plan", map[string]any{"plan": map[string]any{"name": "Native smoke plan", "goal": "Verify shared evidence and native scheduling", "tasks": []any{
				map[string]any{"name": "Verify source", "goal": "Confirm the first result and save evidence", "identifier": "source", "depends_on": []string{}},
				map[string]any{"name": "Verify dependent", "goal": "Use the first accepted result to confirm the second result", "identifier": "dependent", "depends_on": []string{"source"}},
			}}, "plan_document": "# Native smoke plan\nVerify two dependent tasks, review both and write the report."})
		}
		if strings.Contains(prompt, "阶段：PLAN") {
			return nativeResponse(c, req, "submit_plan", map[string]any{})
		}
		matches := taskPattern.FindAllStringSubmatch(prompt, -1)
		if len(matches) < 2 {
			return nil, fmt.Errorf("PLAN STATUS did not expose the executable tasks")
		}
		for _, m := range matches {
			if m[2] == "awaiting_review" {
				modelReviews++
				id, _ := strconv.ParseUint(m[3], 10, 64)
				return nativeResponse(c, req, "review_task", map[string]any{"task_id": m[1], "attempt_id": id, "decision": "accept", "reason": "The delivered worker result and session evidence confirm this task."})
			}
		}
		for _, m := range matches {
			if m[2] == "running" {
				return nativeResponse(c, req, "directly_answer", map[string]any{"answer_payload": "检查新发现，等待任务结算。"})
			}
		}
		for _, m := range matches {
			if m[2] == "pending" {
				return nativeResponse(c, req, "wait_messages", map[string]any{})
			}
		}
		if !saved {
			saved = true
			return nativeResponse(c, req, "save_evidence", map[string]any{"evidence_id": "smoke.coordinator", "evidence_content": "Both dependent tasks were inspected and accepted; their evidence is shared with the session."})
		}
		if !strings.Contains(prompt, "当前任务图和关键消息已经收尾") {
			return nativeResponse(c, req, "directly_answer", map[string]any{"answer_payload": "核对新增事实，任务已审核。"})
		}
		if !reported {
			reported = true
			return nativeResponse(c, req, "create_report", map[string]any{"title": "Native coordinator smoke", "document": "# Native coordinator smoke\nTwo dependent tasks executed and were individually accepted. Shared session evidence was saved."})
		}
		return nativeResponse(c, req, "submit_report", map[string]any{"summary": "Two tasks passed acceptance."})
	}
	record := func(op aicommon.AIEngineOperator, e *schema.AiOutputEvent) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		copy := *e
		copy.Content = append([]byte(nil), e.Content...)
		events = append(events, &copy)
		if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			var payload struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(e.Content, &payload); err != nil {
				approvalErr = err
				return
			}
			approvalErr = op.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: payload.ID, InteractiveJSONInput: `{"suggestion":"continue"}`})
		}
	}
	err := aiengine.InvokeReAct("创建依赖任务，读取本地来源，保存 Evidence、验收并交付报告。", aiengine.WithContext(ctx), aiengine.WithPlanEngine("coordinator"), aiengine.WithMaxIteration(40), aiengine.WithTimeout(60), aiengine.WithReviewPolicy("yolo"), aiengine.WithAICallback(model), aiengine.WithOnEvent(record), aiengine.WithWorkdir(workdir), aiengine.WithExtOptions(aicommon.WithDisableSessionTitleGeneration(true), aicommon.WithAICallback(model), aicommon.WithEnableFunctionCallMode(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithGenerateReport(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithForceManualPlanReview(true)))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		for _, event := range events {
			if event.Type == schema.EVENT_TYPE_END_PLAN_AND_EXECUTION {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond, "PLAN route must close after the coordinator finishes")
	mu.Lock()
	defer mu.Unlock()
	eventsMu.Lock()
	defer eventsMu.Unlock()
	require.NoError(t, approvalErr)
	var coordinatorID, reportPath string
	pushes, pops := 0, 0
	loops := make(map[string]int)
	approved := false
	for _, e := range events {
		var data map[string]any
		_ = json.Unmarshal(e.Content, &data)
		if e.Type == schema.EVENT_TYPE_START_PLAN_AND_EXECUTION {
			coordinatorID, _ = data["coordinator_id"].(string)
		}
		if e.Type == schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE {
			require.False(t, approved, "only one user submission is needed")
			approved = true
			require.Equal(t, true, data["force_manual_review"])
			require.Equal(t, coordinatorID, e.CoordinatorId)
			require.NotEmpty(t, data["id"])
			require.NotEmpty(t, data["selectors"])
		}
		if e.NodeId == "system" {
			if data["type"] == "push_task" {
				pushes++
				require.True(t, approved)
			}
			if data["type"] == "pop_task" {
				pops++
			}
		}
		if e.NodeId == "loop_marker" && data["marker"] == "enter" {
			name, _ := data["loop_name"].(string)
			loops[name]++
		}
		if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
			require.Equal(t, "report-finish", e.NodeId)
			reportPath, _ = data["report_path"].(string)
		}
	}
	require.NotEmpty(t, coordinatorID)
	require.Equal(t, 2, pushes)
	require.Equal(t, 2, pops)
	require.Equal(t, map[string]int{coordinator.Name: 1, "pe_task": 2}, loops)
	require.NotEmpty(t, reportPath)
	_, err = os.Stat(reportPath)
	require.NoError(t, err)
	require.GreaterOrEqual(t, workerCalls, 6)
	require.Equal(t, 2, modelReviews, "both real results must still be reviewed")
	require.LessOrEqual(t, coordinatorCalls, 12, "notifications must not introduce empty model polling")
	combined := strings.Join(prompts, "\n")
	require.Contains(t, combined, "smoke.task.")
	require.Contains(t, combined, "smoke.coordinator")
	require.Contains(t, combined, "coordinator-exploration-sentinel")
	t.Logf("Yak + aim native smoke passed: coordinator calls=%d, worker calls=%d, loops=%v, pushes=%d, pops=%d, report written", coordinatorCalls, workerCalls, loops, pushes, pops)
}
