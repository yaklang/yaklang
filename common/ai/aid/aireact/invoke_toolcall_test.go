package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedToolCalling(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_ToolUse(t *testing.T) {
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			sleepInt := params.GetFloat("seconds", 0.3)
			if sleepInt <= 0 {
				sleepInt = 0.3
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	reviewed := false
	reviewReleased := false
	toolCallOutputEvent := false
	var iid string
LOOP:
	for {
		select {
		case e := <-out:
			if e.IsStream {
				if e.ContentType == "" {
					t.Fatal("stream event should have content type")
				}
				if utils.IsNil(e.GetNodeIdVerbose()) {
					t.Fatal("node id should not be nil")
				}
				fmt.Println(string(e.GetStreamDelta()))
			}
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.Type == string(schema.EVENT_TYPE_REVIEW_RELEASE) {
				gotId := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				if gotId == iid {
					reviewReleased = true
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallOutputEvent = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	if !toolCalled {
		t.Fatal("Tool was not called")
	}

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one output event, but got none")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !strings.Contains(tl, `mocked thought for tool calling`) {
		t.Fatal("timeline does not contain mocked thought")
	}
	if !utils.MatchAllOfSubString(tl, `system-question`, "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}
	if !utils.MatchAllOfSubString(tl, `ReAct iteration 1`, `ReAct Iteration Done[1]`) {
		t.Fatal("timeline does not contain ReAct iteration")
	}
	fmt.Println("--------------------------------------")
}

func TestReAct_ToolUse_WithCallbackRuntimeID(t *testing.T) {
	t.Skip("这是一个应该被改进的测试，需要更细粒度的id控制： task id 管理多个 call tool id")
	// todo  这是一个应该被改进的测试，需要更细粒度的id控制： task id 管理多个 call tool id
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	var capturedRuntimeID string

	toolCalled := false
	testTool, err := aitool.New(
		"test_tool",
		aitool.WithNumberParam("value"),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			if runtimeConfig != nil {
				capturedRuntimeID = runtimeConfig.RuntimeID
			}
			return "test result", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "test_tool")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test input",
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var taskID string
	var iid string
LOOP:
	for {
		select {
		case e := <-out:
			// Capture task ID from task creation or status change events
			if e.NodeId == "react_task_created" || e.NodeId == "react_task_status_changed" {
				if tid := jsonpath.FindFirst(e.GetContent(), "$..react_task_id"); tid != nil {
					taskID = utils.InterfaceToString(tid)
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCalled {
		t.Fatal("Tool was not called")
	}

	// Verify that the RuntimeID in the tool callback matches the task ID
	if capturedRuntimeID == "" {
		t.Fatal("RuntimeID was not captured in tool callback")
	}

	if taskID == "" {
		t.Fatal("Task ID is empty - task ID was not captured from events")
	}

	if capturedRuntimeID != taskID {
		t.Fatalf("RuntimeID mismatch: expected %s, got %s", taskID, capturedRuntimeID)
	}

	fmt.Printf("✓ RuntimeID matches Task ID: %s\n", taskID)
}

func TestReAct_ToolUse_WithNoToolsCache(t *testing.T) {
	// 注册 YakScript 工具转换函数（模拟 yak 包的 init 函数）
	yakscripttools.RegisterYakScriptAiToolsCovertHandle(func(aitools []*schema.AIYakTool) []*aitool.Tool {
		tools := []*aitool.Tool{}
		for _, aiTool := range aitools {
			tool := mcp.NewTool(aiTool.Name)
			tool.Description = aiTool.Description
			dataMap := map[string]any{}
			err := json.Unmarshal([]byte(aiTool.Params), &dataMap)
			if err != nil {
				log.Errorf("unmarshal aiTool.Params failed: %v", err)
				continue
			}
			tool.InputSchema.FromMap(dataMap)
			at, err := aitool.NewFromMCPTool(
				tool,
				aitool.WithDescription(aiTool.Description),
				aitool.WithKeywords(strings.Split(aiTool.Keywords, ",")),
				aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
					// 简单的测试回调
					return "test tool executed successfully", nil
				}),
			)
			if err != nil {
				log.Errorf("create aitool failed: %v", err)
				continue
			}
			tools = append(tools, at)
		}
		return tools
	})

	// 生成一个唯一的工具名（UUID）
	toolName := "test_sleep_" + ksuid.New().String()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	_, err := NewTestReAct(
		aicommon.WithAiToolManagerOptions(buildinaitools.WithNoToolsCache(), buildinaitools.WithEnableAllTools()),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 确保数据库表结构正确
	db := consts.GetGormProfileDatabase()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}

	// 第一次尝试：工具不存在，应该失败
	fmt.Printf("Phase 1: Attempting to call non-existent tool '%s'\n", toolName)
	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "please use " + toolName,
	}

	after := time.After(du * time.Second)
	firstTaskFailed := false
	firstTaskCompleted := false

LOOP1:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				// 任务执行失败标志
				if utils.InterfaceToString(result) == "aborted" {
					firstTaskFailed = true
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				// 任务执行完成标志
				if utils.InterfaceToString(result) == "completed" {
					firstTaskCompleted = true
					break LOOP1
				}
			}
		case <-after:
			break LOOP1
		}
	}

	if !(firstTaskCompleted && firstTaskFailed) {
		t.Fatal("first task call failed")
	}

	// 创建工具
	fmt.Printf("\nPhase 2: Creating tool '%s'\n", toolName)
	newTool := &schema.AIYakTool{
		Name:        toolName,
		VerboseName: "Test Sleep Tool",
		Description: "A test tool that simulates sleep operation",
		Keywords:    "test,sleep,dynamic",
		Content: `# Test Sleep Tool
yakit.AutoInitYakit()

cli.Float("seconds", cli.setDefault(0.1), cli.setHelp("sleep duration in seconds"))

seconds = cli.Float("seconds")
sleep(seconds)
println("Slept for", seconds, "seconds")
`,
		Params: `{"type":"object","properties":{"seconds":{"type":"number","description":"sleep duration in seconds","default":0.1}}}`,
		Path:   "test/sleep",
	}

	_, err = yakit.CreateAIYakTool(db, newTool)
	if err != nil {
		t.Fatalf("Failed to create AI yak tool: %v", err)
	}
	fmt.Printf("✓ Created AI Yak Tool: %s\n", toolName)

	// 第二次尝试：工具已存在，应该成功
	fmt.Printf("\nPhase 3: Attempting to call newly created tool '%s'\n", toolName)
	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "please use " + toolName,
	}

	after = time.After(du * time.Second)
	secondTaskCompleted := false
	toolReviewReleased := false
	toolCallDone := false
LOOP2:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

			// 检查工具是否被成功调用
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				fmt.Printf("✓ Tool '%s' executed successfully\n", toolName)
				toolCallDone = true
			}

			// 检查 prompt 中是否包含工具名
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				if strings.Contains(string(e.Content), toolName) {
					fmt.Printf("✓ Tool '%s' found in tool list (after creation)\n", toolName)
					toolReviewReleased = true
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					secondTaskCompleted = true
					break LOOP2
				}
			}
		case <-after:
			break LOOP2
		}
	}

	close(in)

	if !(secondTaskCompleted && toolReviewReleased && toolCallDone) {
		t.Fatal("second task call")
	}

	// 清理：删除创建的工具
	err = db.Where("name = ?", toolName).Delete(&schema.AIYakTool{}).Error
	if err != nil {
		t.Logf("Warning: failed to cleanup tool: %v", err)
	}

	fmt.Printf("\n✓ Test completed: WithNoToolsCache allows dynamic tool loading\n")
}
