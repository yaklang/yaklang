package aireact

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_PlanAndExecute_InheritsDistinctCallbacks verifies that when the
// outer ReAct triggers plan-and-execute, the child Coordinator inherits the
// parent's DISTINCT Quality/Speed/Original callbacks rather than re-deriving
// them via WithAutoTieredAICallback (which would silently fall back to the
// lightweight model when the intelligent model fails to load from global config).
//
// Setup:
//   - qualityCb  → model "quality-inherited"  (QualityPriority)
//   - speedCb    → model "speed-light"        (SpeedPriority)
//   - originalCb → model "original-base"      (Original, set via WithAICallback)
//
// Expected: inside plan execution, Config.CallAI picks QualityPriority first,
// so AI_CALL_SUMMARY events after plan start must show "quality-inherited".
// "original-base" must NOT appear (it would mean QualityPriority was lost and
// CallAI fell through to a callback derived from Original).
func TestReAct_PlanAndExecute_InheritsDistinctCallbacks(t *testing.T) {
	const qualityModel = "quality-inherited"
	const speedModel = "speed-light"
	const originalModel = "original-base"

	var qualityAfterPlan int32
	var speedAfterPlan int32
	var originalAfterPlan int32
	var planStarted int32

	mockTool, err := aitool.New(
		"mock_callback_inherit_tool",
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			_, _ = io.WriteString(stdout, "tool ok")
			return map[string]any{"ok": true}, nil
		}),
	)
	require.NoError(t, err)

	allPromptsHandler := func(modelName string, afterPlan *int32) aicommon.AICallbackType {
		return func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if atomic.LoadInt32(&planStarted) > 0 {
				atomic.AddInt32(afterPlan, 1)
			}

			rsp := i.NewAIResponse()
			rsp.SetModelInfo("mock", modelName)
			prompt := req.GetPrompt()

			switch {
			// Outer ReAct: first free-input → trigger plan-and-execute
			case utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_"):
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "execute callback inherit test" },
"human_readable_thought": "delegate", "cumulative_summary": "delegate to plan execution"}
`))

			// Plan LiteForge: generate plan from document (action name = plan_from_document)
			case utils.MatchAllOfSubString(prompt, "main_task", "main_task_goal", "subtask_name", "subtask_goal"):
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "plan_from_document",
  "main_task": "callback inherit test",
  "main_task_goal": "run mock tool and verify callback inheritance",
  "tasks": [
    {
      "subtask_name": "call mock tool",
      "subtask_goal": "invoke mock_callback_inherit_tool"
    }
  ]
}`))

			// Inner: subtask ReAct loop → require tool
			case utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_", "directly_answer", "require_tool"):
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "mock_callback_inherit_tool" },
"human_readable_thought": "call tool", "cumulative_summary": "call tool"}
`))

			// Inner: tool parameter generation
			case utils.MatchAllOfSubString(prompt, "Generate appropriate parameters for this tool call based on the context above"):
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-tool", "tool": "mock_callback_inherit_tool", "params": {}}
`))

			// Inner: task progress decision
			case utils.MatchAllOfSubString(prompt, "continue-current-task", "proceed-next-task", "task-failed"):
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "continue-current-task",
  "status_summary": "tool call completed",
  "task_short_summary": "mock tool done"
}`))

			// Inner: task summary
			case utils.MatchAllOfSubString(prompt, "任务执行引擎", "task_long_summary") &&
				!utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_"):
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "summary",
  "status_summary": "done",
  "task_short_summary": "completed",
  "task_long_summary": "mock tool executed"
}`))

			// Verification (both outer and inner)
			case utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning"):
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "verify-satisfaction",
  "user_satisfied": true,
  "reasoning": "task completed"
}`))

			// Outer / inner: final answer
			case utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") &&
				!utils.MatchAllOfSubString(prompt, "require_tool"):
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "directly_answer",
  "answer_payload": "done"
}`))

			default:
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "directly_answer",
  "answer_payload": "fallback"
}`))
			}

			rsp.Close()
			return rsp, nil
		}
	}

	qualityCb := allPromptsHandler(qualityModel, &qualityAfterPlan)
	speedCb := allPromptsHandler(speedModel, &speedAfterPlan)
	originalCb := allPromptsHandler(originalModel, &originalAfterPlan)

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 200)

	_, err = NewTestReAct(
		// Step 1: set all three callbacks to originalCb (with wrappers)
		aicommon.WithAICallback(originalCb),
		// Step 2: override Quality with DISTINCT qualityCb
		aicommon.WithQualityPriorityAICallback(qualityCb),
		// Step 3: override Speed with DISTINCT speedCb
		aicommon.WithSpeedPriorityAICallback(speedCb),
		// After this:
		//   QualityPriority = wrapper(qualityCb)  → "quality-inherited"
		//   SpeedPriority   = wrapper(speedCb)    → "speed-light"
		//   Original        = originalCb          → "original-base"
		// CallAI tries Quality→Speed→Original, so Quality wins.
		// OLD BUG: invokePlanAndExecute used WithAutoTieredAICallback(Original)
		//          which would make child use Original for everything.
		// FIX: invokePlanAndExecute now inherits parent's callbacks directly.
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithTools(mockTool),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "run callback inherit test",
	}

	var (
		mu                   sync.Mutex
		modelsAfterPlan      []string
		sawQualityAfterPlan  bool
		sawOriginalAfterPlan bool
		reactTaskCompleted   bool
	)

	timeout := time.After(30 * time.Second)
LOOP:
	for {
		select {
		case e := <-out:
			switch e.Type {
			case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
				atomic.StoreInt32(&planStarted, 1)

			case schema.EVENT_TYPE_AI_CALL_SUMMARY:
				var data map[string]any
				if err := json.Unmarshal(e.Content, &data); err == nil {
					modelName := utils.InterfaceToString(data["model_name"])
					if atomic.LoadInt32(&planStarted) > 0 {
						mu.Lock()
						modelsAfterPlan = append(modelsAfterPlan, modelName)
						if modelName == qualityModel {
							sawQualityAfterPlan = true
						}
						if modelName == originalModel {
							sawOriginalAfterPlan = true
						}
						mu.Unlock()
					}
				}

			case schema.EVENT_TYPE_STRUCTURED:
				if e.NodeId == "react_task_status_changed" {
					var data map[string]any
					if err := json.Unmarshal(e.Content, &data); err == nil {
						if utils.InterfaceToString(data["react_task_now_status"]) == "completed" {
							reactTaskCompleted = true
							break LOOP
						}
					}
				}
			}
		case <-timeout:
			t.Log("timeout waiting for task completion")
			break LOOP
		}
	}

	t.Logf("quality calls after plan: %d", atomic.LoadInt32(&qualityAfterPlan))
	t.Logf("speed calls after plan: %d", atomic.LoadInt32(&speedAfterPlan))
	t.Logf("original calls after plan: %d", atomic.LoadInt32(&originalAfterPlan))
	t.Logf("models after plan start: %v", modelsAfterPlan)

	require.True(t, reactTaskCompleted, "expected delegated ReAct task to complete")

	// Quality callback must be used inside plan execution (via Config.CallAI → QualityPriority).
	require.True(t, sawQualityAfterPlan,
		"expected quality model %q after plan start, got: %v", qualityModel, modelsAfterPlan)

	// Original model must NOT appear after plan start.
	// If it does, it means the child Coordinator lost QualityPriority and fell back
	// to a callback derived from OriginalAICallback — which is exactly the old bug.
	require.False(t, sawOriginalAfterPlan,
		"original model %q must NOT appear after plan start (would mean Quality callback was lost), got: %v",
		originalModel, modelsAfterPlan)

	require.Greater(t, atomic.LoadInt32(&qualityAfterPlan), int32(0),
		"quality callback must be called inside plan execution")
}
