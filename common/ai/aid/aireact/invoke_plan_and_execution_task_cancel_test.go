package aireact

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_PlanAndExecute_TaskCancel(t *testing.T) {
	// mock 一个 forge ，用于测试取消 pe 任务
	testForgeName := "test_forge_" + utils.RandStringBytes(16)
	planFlag := "plan_flag_" + utils.RandStringBytes(16)

	// 调试日志：打印测试配置
	t.Logf("[TEST] TestReAct_PlanAndExecute_TaskCancel started")
	t.Logf("[TEST] testForgeName: %s", testForgeName)
	t.Logf("[TEST] planFlag: %s", planFlag)
	forge := &schema.AIForge{
		ForgeName:    testForgeName,
		ForgeType:    "yak",
		ForgeContent: "",
		InitPrompt: `{{ if .Forge.UserParams }}
## 分析任务参数
<content_wait_for_review>
{{ .Forge.UserParams }}
</content_wait_for_review>
{{end}}
**分析目标**: {{ .Forge.UserQuery }}`,
		PlanPrompt: `{
  "@action": "plan",
  "query": "-",
  "main_task": "` + planFlag + `",
  "main_task_goal": "通过系统化的分析方法，从海量告警中识别真实安全威胁，过滤误报和噪声，生成详细的降噪分析报告，帮助安全团队提高告警处理效率。",
  "tasks": [
    {
      "subtask_name": "xxx",
      "subtask_goal": "xxx"
    }
  ]
}`,
	}
	yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge)
	defer func() {
		yakit.DeleteAIForge(consts.GetGormProfileDatabase(), &ypb.AIForgeFilter{
			ForgeName: testForgeName,
		})
	}()

	// REACT 	输入输出
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	var reactIns *ReAct

	_ = reactIns
	// 用户解析任务取消结果
	syncID := ksuid.New().String()

	var firstCall bool             // 是否是第一次调用工具
	var callToolTwice bool         // 是否是第二次调用工具，预期工具只调用一次
	var unreachableCode bool       // 是否到达了非预期的代码
	var currentTaskID string       // 当前任务ID, 用于取消任务
	var cancelTaskSuccess bool     // 取消任务是否成功
	var callAIAfterCancelTask bool // 是否在取消任务后调用 AI
	// var contextCanceledErr bool    // 上下文取消错误
	waitCancelTaskDone := make(chan struct{}, 1)
	// mock 工具，用于取消任务
	mockToolName := "mock_cancel_tool_" + utils.RandStringBytes(16)
	t.Logf("[TEST] mockToolName: %s", mockToolName)

	// 用于在闭包中收集日志
	var callbackLogs []string
	addLog := func(format string, args ...any) {
		callbackLogs = append(callbackLogs, fmt.Sprintf(format, args...))
	}

	mockCancelTool, err := aitool.New(
		mockToolName,
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			addLog("[MOCK_TOOL] Mock tool '%s' invoked, firstCall=%v", mockToolName, firstCall)
			if firstCall {
				addLog("[MOCK_TOOL] Tool called twice!")
				callToolTwice = true
				return "", nil
			}

			firstCall = true
			addLog("[MOCK_TOOL] Sending cancel task event for taskID=%s", currentTaskID)
			in <- &ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_REACT_CANCEL_TASK,
				SyncJsonInput: `{"task_id":"` + currentTaskID + `"}`,
				SyncID:        syncID,
			}

			addLog("[MOCK_TOOL] Waiting for cancel task done signal...")
			<-waitCancelTaskDone
			addLog("[MOCK_TOOL] Cancel task done signal received")
			cancelTaskSuccess = true
			return "", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if cancelTaskSuccess {
				addLog("[AI_CALLBACK] AI callback invoked after task was cancelled!")
				callAIAfterCancelTask = true
			}
			prompt := r.GetPrompt()
			addLog("[AI_CALLBACK] ========== AI callback invoked ==========")
			addLog("[AI_CALLBACK] Full prompt:\n%s", prompt)
			addLog("[AI_CALLBACK] ========== End of prompt ==========")

			// ReAct 主循环的响应 - 请求 blueprint (forge)
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool") {
				addLog("[AI_CALLBACK] MATCHED: ReAct main loop (directly_answer, require_ai_blueprint, require_tool)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
		{"@action": "object", "next_action": { "type": "require_ai_blueprint", "blueprint_payload": "` + testForgeName + `" },
		"human_readable_thought": "requesting forge to analyze vulnerability", "cumulative_summary": "forge analysis"}
		`))
				rsp.Close()
				return rsp, nil
			}

			// Blueprint 参数生成
			if utils.MatchAllOfSubString(prompt, "Blueprint Schema:", "Blueprint Description:", "call-ai-blueprint") {
				addLog("[AI_CALLBACK] MATCHED: Blueprint parameter generation (Blueprint Schema, call-ai-blueprint)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
		{"@action": "call-ai-blueprint","blueprint": "` + testForgeName + `", "params": {"target": "http://example.com", "query": "` + "abc" + `"},
		"human_readable_thought": "generating blueprint parameters (AI rewrote the query)", "cumulative_summary": "forge parameters"}
		`))
				rsp.Close()
				return rsp, nil
			}

			// 处理 plan prompt - 任务规划
			// Plan prompt 的关键标识: 包含 "任务规划使命" 或 "任务设计输出要求" 或 "OUTPUT_EXAMPLE_END"
			isPlanRequest := (strings.Contains(prompt, "任务规划使命") || strings.Contains(prompt, "你是一个输出JSON的任务规划的工具")) &&
				(strings.Contains(prompt, "任务设计输出要求") || strings.Contains(prompt, "OUTPUT_EXAMPLE_END"))
			if isPlanRequest {
				addLog("[AI_CALLBACK] MATCHED: Plan prompt (任务规划)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "plan",
  "query": "-",
  "main_task": "` + planFlag + `",
  "main_task_goal": "执行测试任务",
  "tasks": [
    {
      "subtask_name": "执行子任务",
      "subtask_goal": "完成测试"
    }
  ]
}`))
				rsp.Close()
				return rsp, nil
			}

			// 检查是否匹配 planFlag（但不包含 call-tool）
			hasPlanFlag := strings.Contains(prompt, planFlag)
			hasCallTool := strings.Contains(prompt, "call-tool")
			hasToolnameYet := strings.Contains(prompt, "toolname_yet")
			addLog("[AI_CALLBACK] Check conditions - planFlag(%s): %v, call-tool: %v, toolname_yet: %v", planFlag, hasPlanFlag, hasCallTool, hasToolnameYet)

			if hasPlanFlag && !hasCallTool && !hasToolnameYet {
				addLog("[AI_CALLBACK] MATCHED: Task requires tool (planFlag found, no call-tool)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "require_tool", "tool_require_payload": "` + mockToolName + `", 
"human_readable_thought": "为了取消当前任务，我需要使用` + mockToolName + `工具。当前任务明确要求使用该工具，无需其他参数，直接执行即可取消当前任务。"}
		`))
				rsp.Close()
				return rsp, nil
			}

			if hasCallTool && hasToolnameYet {
				addLog("[AI_CALLBACK] MATCHED: Call tool (call-tool + toolname_yet)")
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{
  "@action": "call-tool",
  "tool": "` + mockToolName + `",
  "params": {}
}
		`))
				rsp.Close()
				return rsp, nil
			}

			addLog("[AI_CALLBACK] NO MATCH - unreachable code path")
			unreachableCode = true
			return nil, errors.New("unreachable code")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithTools(mockCancelTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "帮我调主机扫描蓝图扫描我的主机",
		}
	}()

	after := time.After(30 * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			addLog("[EVENT] Received event: Type=%s, NodeId=%s", e.Type, e.NodeId)
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				addLog("[EVENT] Plan execution started")
			}

			if e.GetIsSync() && e.GetSyncID() == syncID && e.NodeId == string("react_task_cancelled") {
				addLog("[EVENT] Task cancelled event received, sending done signal")
				waitCancelTaskDone <- struct{}{}
			}
			if e.Type == string(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC) {
				task_id := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..task_id"))
				currentTaskID = task_id
				addLog("[EVENT] Task switched to async, taskID=%s", currentTaskID)
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				addLog("[EVENT] Plan execution ended")
				break LOOP
			}

		case <-after:
			addLog("[EVENT] Test timeout after 30s")
			break LOOP
		}
	}
	close(in)

	// 打印所有收集的日志（只有测试失败时才会显示）
	t.Logf("[TEST] ========== Collected logs ==========")
	for _, logLine := range callbackLogs {
		t.Logf("%s", logLine)
	}
	t.Logf("[TEST] ========== End of logs ==========")

	t.Logf("[TEST] Final state: firstCall=%v, callToolTwice=%v, unreachableCode=%v, cancelTaskSuccess=%v, callAIAfterCancelTask=%v, currentTaskID=%s",
		firstCall, callToolTwice, unreachableCode, cancelTaskSuccess, callAIAfterCancelTask, currentTaskID)

	if !firstCall {
		t.Fatalf("firstCall is false - mock tool '%s' was never invoked (expected planFlag=%s to trigger require_tool)", mockToolName, planFlag)
	}
	if callToolTwice {
		t.Fatal("call tool twice")
	}
	if unreachableCode {
		t.Fatal("unreachable code - AI callback hit unexpected branch")
	}
	if !cancelTaskSuccess {
		t.Fatal("cancel task failed")
	}
	if callAIAfterCancelTask {
		t.Fatal("call AI after cancel task should not be called")
	}
	// if !contextCanceledErr {
	// 	t.Fatal("context canceled error not found")
	// }
}

func TestReAct_CancelTask_InLoop(t *testing.T) {
	// REACT 	输入输出
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	var reactIns *ReAct

	_ = reactIns
	// 用户解析任务取消结果
	syncID := ksuid.New().String()

	var firstCall bool       // 是否是第一次调用工具
	var unreachableCode bool // 是否到达了非预期的代码
	waitCancelTaskDone := make(chan struct{}, 1)
	// mock 工具，用于取消任务
	mockToolName := "mock_cancel_tool_" + utils.RandStringBytes(16)
	mockCancelTool, err := aitool.New(
		mockToolName,
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			firstCall = true
			reactIns.GetCurrentTask().Cancel()
			return "", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err = NewTestReAct(
		// aicommon.WithAICallback(aiCallback),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()

			if !utils.MatchAllOfSubString(prompt, "call-tool", "toolname_yet") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "require_tool", "tool_require_payload": "` + mockToolName + `", 
"human_readable_thought": "为了取消当前任务，我需要使用` + mockToolName + `工具。当前任务明确要求使用该工具，无需其他参数，直接执行即可取消当前任务。"}
		`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "call-tool", "toolname_yet") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{
  "@action": "call-tool",
  "tool": "` + mockToolName + `",
  "params": {}
}
		`))
				rsp.Close()
				return rsp, nil
			}
			unreachableCode = true
			return nil, errors.New("unreachable code")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithTools(mockCancelTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "帮我调主机扫描蓝图扫描我的主机",
		}
	}()

	after := time.After(30000 * time.Second)

	var taskAborted bool

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				t.Log("plan execution started")
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				status := utils.InterfaceToString(result)
				if status == "aborted" {
					taskAborted = true
					break LOOP
				}
			}
			if e.GetIsSync() {
				t.Log("is sync")
			}
			if e.GetIsSync() && e.GetSyncID() == syncID && e.NodeId == string("react_task_cancelled") {
				waitCancelTaskDone <- struct{}{}
			}

		case <-after:
			t.Log("test timeout")
			break LOOP
		}
	}
	close(in)

	if !firstCall {
		t.Fatal("first call failed")
	}
	if unreachableCode {
		t.Fatal("unreachable code")
	}

	if !taskAborted {
		t.Fatal("task aborted")
	}
}
