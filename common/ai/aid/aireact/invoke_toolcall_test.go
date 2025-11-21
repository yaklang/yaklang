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
	"github.com/yaklang/yaklang/common/yak"
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

func TestReAct_ToolUse_TaskGetRisks(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 生成唯一标识符用于验证
	flagTitle := ksuid.New().String()
	flagTarget := "http://test-" + ksuid.New().String() + ".example.com"

	// 创建一个生成 risk 的测试工具
	testToolScript := `
yakit.AutoInitYakit()
target = cli.String("target", cli.setRequired(true))
title = cli.String("title", cli.setRequired(true))
cli.check()

risk.NewRisk(
	target,
	risk.title(title),
	risk.type("baseline"),
	risk.severity("low"),
	risk.description("This is a test risk created by TestReAct_ToolUse_WithCallbackRuntimeID"),
)
`

	// Params 字段应该是 JSON Schema 格式
	paramsJSON := `{
		"type": "object",
		"properties": {
			"target": {
				"type": "string",
				"description": "The target URL or IP address"
			},
			"title": {
				"type": "string",
				"description": "The title of the risk"
			}
		},
		"required": ["target", "title"]
	}`

	tools := yak.YakTool2AITool([]*schema.AIYakTool{
		{
			Name:        "create_test_risk",
			Description: "Create a test security risk",
			Content:     testToolScript,
			Params:      paramsJSON,
		},
	})
	if len(tools) == 0 {
		t.Fatal("tools not found")
	}

	riskTool := tools[0]
	if riskTool == nil {
		t.Fatal("risk tool not found")
	}

	// 创建 ReAct 实例
	reAct, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			// Mock AI 响应，让它调用 create_test_risk 工具
			prompt := r.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "create_test_risk" },
"human_readable_thought": "I need to create a test risk", "cumulative_summary": "Creating test risk"}
`))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				// 返回工具参数
				paramsJSON := fmt.Sprintf(`{"@action": "call-tool", "params": { "target": "%s", "title": "%s" }}`, flagTarget, flagTitle)
				rsp.EmitOutputStream(bytes.NewBufferString(paramsJSON))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "test risk created successfully"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(riskTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 发送输入事件
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "create a test risk",
		}
	}()

	du := time.Duration(10)
	// if utils.InGithubActions() {
	// 	du = time.Duration(5)
	// }
	after := time.After(du * time.Second)

	var taskID string
	var iid string
	toolCalled := false
	taskCompleted := false

LOOP:
	for {
		select {
		case e := <-out:
			// 捕获 task ID
			if e.NodeId == "react_task_created" || e.NodeId == "react_task_status_changed" {
				if tid := jsonpath.FindFirst(e.GetContent(), "$..react_task_id"); tid != nil {
					taskID = utils.InterfaceToString(tid)
					fmt.Printf("Captured Task ID: %s\n", taskID)
				}
			}

			// 处理工具审核请求
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			// 检测工具是否被调用
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCalled = true
				fmt.Println("Tool call completed")
			}

			// 检测任务完成
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				log.Infof("task status: %s", utils.InterfaceToString(result))
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					fmt.Println("Task completed")
					break LOOP
				}
			}
		case <-after:
			t.Log("Test timeout")
			break LOOP
		}
	}
	close(in)

	// 验证工具被调用
	if !toolCalled {
		t.Fatal("Tool was not called")
	}

	// 验证任务完成
	if !taskCompleted {
		t.Fatal("Task did not complete")
	}

	// 验证 Task ID 被捕获
	if taskID == "" {
		t.Fatal("Task ID was not captured from events")
	}

	// 获取最后一个任务
	lastTask := reAct.GetLastTask()
	if lastTask == nil {
		t.Fatal("last task not found")
	}

	// 验证 Task ID 匹配
	if lastTask.GetId() != taskID {
		t.Fatalf("Task ID mismatch: expected %s, got %s", taskID, lastTask.GetId())
	}

	fmt.Printf("✓ Task ID verified: %s\n", taskID)

	// 通过 GetRisks 方法获取创建的 risks
	risks := reAct.GetLastTask().GetRisks()
	if len(risks) == 0 {
		t.Fatal("No risks found")
	}

	// risks 数量为 1
	if len(risks) != 1 {
		t.Fatalf("Expected 1 risk, got %d", len(risks))
	}

	// 验证 risk 的内容
	found := false
	for _, risk := range risks {
		fmt.Printf("Found risk: ID=%d, Title=%s, Target=%s, RuntimeID=%s\n",
			risk.ID, risk.Title, risk.Url, risk.RuntimeId)
		if risk.Title == flagTitle && risk.Url == flagTarget {
			found = true
			// 验证 RuntimeID 是否被正确设置
			if risk.RuntimeId == "" {
				t.Error("Risk RuntimeID is empty")
			}
			fmt.Printf("✓ Risk verified: Title=%s, Target=%s, RuntimeID=%s\n",
				risk.Title, risk.Url, risk.RuntimeId)
			break
		}
	}

	if !found {
		t.Fatalf("Expected risk not found: title=%s, target=%s", flagTitle, flagTarget)
	}

	fmt.Println("✓ Test completed successfully")
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
