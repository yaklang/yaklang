package aireact

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_PlanAndExecute_TaskCancel(t *testing.T) {
	// mock 一个 forge ，用于测试取消 pe 任务
	testForgeName := "test_forge_" + utils.RandStringBytes(16)
	planFlag := "plan_flag_" + utils.RandStringBytes(16)
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
	mockCancelTool, err := aitool.New(
		mockToolName,
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			if firstCall {
				callToolTwice = true
				return "", nil
			}

			firstCall = true
			in <- &ypb.AIInputEvent{
				IsSyncMessage: true,
				SyncType:      SYNC_TYPE_REACT_CANCEL_TASK,
				SyncJsonInput: `{"task_id":"` + currentTaskID + `"}`,
				SyncID:        syncID,
			}

			<-waitCancelTaskDone
			cancelTaskSuccess = true
			return "", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	reactIns, err = NewTestReAct(
		// aicommon.WithAICallback(aiCallback),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if cancelTaskSuccess {
				callAIAfterCancelTask = true
			}
			prompt := r.GetPrompt()
			// ReAct 主循环的响应 - 请求 blueprint (forge)
			if utils.MatchAllOfSubString(prompt, "directly_answer", "require_ai_blueprint", "require_tool") {
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
				// 重要：AI 返回的 query 参数故意不包含用户原始问题，模拟 AI 改写导致信息丢失的情况
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
		{"@action": "call-ai-blueprint","blueprint": "` + testForgeName + `", "params": {"target": "http://example.com", "query": "` + "abc" + `"},
		"human_readable_thought": "generating blueprint parameters (AI rewrote the query)", "cumulative_summary": "forge parameters"}
		`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, planFlag) && !utils.MatchAllOfSubString(prompt, "call-tool", "toolname_yet") {
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

	after := time.After(30 * time.Second)

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION) {
				log.Infof("plan execution started")
			}

			// if e.NodeId == "react_task_status_changed" {
			// 	result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
			// 	status := utils.InterfaceToString(result)
			// 	if status == "completed" || status == "aborted" {
			// 		break LOOP
			// 	}
			// }

			if e.GetIsSync() && e.GetSyncID() == syncID && e.NodeId == string("react_task_cancelled") {
				waitCancelTaskDone <- struct{}{}
			}
			if e.Type == string(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC) {
				task_id := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..task_id"))
				currentTaskID = task_id
			}
			if e.Type == string(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION) {
				break LOOP
			}
			// if e.Type == string(schema.EVENT_TYPE_FAIL_PLAN_AND_EXECUTION) {
			// 	contextCanceledErr = strings.Contains(string(e.Content), "context canceled")
			// }

		case <-after:
			log.Warnf("test timeout")
			break LOOP
		}
	}
	close(in)

	if !firstCall {
		t.Fatal("first call failed")
	}
	if callToolTwice {
		t.Fatal("call tool twice")
	}
	if unreachableCode {
		t.Fatal("unreachable code")
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
				log.Infof("plan execution started")
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
				println("is sync")
			}
			if e.GetIsSync() && e.GetSyncID() == syncID && e.NodeId == string("react_task_cancelled") {
				waitCancelTaskDone <- struct{}{}
			}

		case <-after:
			log.Warnf("test timeout")
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
