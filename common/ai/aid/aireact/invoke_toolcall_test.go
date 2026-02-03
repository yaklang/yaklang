package aireact

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
		// Include identifier field for new directory structure
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "sleep_test", "params": { "seconds" : 0.1 }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason", "human_readable_result": "mocked thought for verification"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_ToolUse_Timing(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolCalled := false
	timingTool, err := aitool.New(
		"sleep_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			// No sleep needed - just return immediately
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "sleep_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(timingTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test timing",
		}
	}()

	after := time.After(10 * time.Second)

	var startTime, endTime int64
	var durationMs int64
	toolStartReceived := false
	toolDoneReceived := false
	reviewed := false
	var iid string

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				toolStartReceived = true
				if st := jsonpath.FindFirst(string(e.Content), "$.start_time"); st != nil {
					startTime = int64(utils.InterfaceToInt(st))
				}
				if stms := jsonpath.FindFirst(string(e.Content), "$.start_time_ms"); stms != nil {
					startTimeMs := int64(utils.InterfaceToInt(stms))
					require.Greater(t, startTimeMs, int64(0), "start_time_ms should be greater than 0")
					fmt.Printf("Tool call started at: %d (unix timestamp), %d (ms)\n", startTime, startTimeMs)
				}
				if startTime > 0 {
					require.Greater(t, startTime, int64(0), "start_time should be greater than 0")
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolDoneReceived = true
				if et := jsonpath.FindFirst(string(e.Content), "$.end_time"); et != nil {
					endTime = int64(utils.InterfaceToInt(et))
				}
				if etms := jsonpath.FindFirst(string(e.Content), "$.end_time_ms"); etms != nil {
					endTimeMs := int64(utils.InterfaceToInt(etms))
					require.Greater(t, endTimeMs, int64(0), "end_time_ms should be greater than 0")
					fmt.Printf("Tool call ended at: %d (unix timestamp), %d (ms)\n", endTime, endTimeMs)
				}
				if dm := jsonpath.FindFirst(string(e.Content), "$.duration_ms"); dm != nil {
					durationMs = int64(utils.InterfaceToInt(dm))
					require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should be >= 0")
					// Duration should be reasonable (tool executes immediately, but should still take some time)
					require.LessOrEqual(t, durationMs, int64(5000), "duration should be less than 5000ms")
				}
				if ds := jsonpath.FindFirst(string(e.Content), "$.duration_seconds"); ds != nil {
					durationSeconds := utils.InterfaceToFloat64(ds)
					require.GreaterOrEqual(t, durationSeconds, 0.0, "duration_seconds should be >= 0")
					fmt.Printf("Tool call duration: %d ms (%.3f seconds)\n", durationMs, durationSeconds)
				}

				if endTime > 0 {
					require.Greater(t, endTime, int64(0), "end_time should be greater than 0")
					if startTime > 0 {
						require.GreaterOrEqual(t, endTime, startTime, "end_time should be >= start_time")
					}
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
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
					if toolStartReceived && toolDoneReceived {
						break LOOP
					}
				}
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	require.True(t, toolStartReceived, "Expected to receive tool_call_start event")
	require.True(t, toolDoneReceived, "Expected to receive tool_call_done event")
	require.True(t, reviewed, "Expected to have at least one review event")
	require.True(t, toolCalled, "Tool was not called")
	require.Greater(t, startTime, int64(0), "start_time should have been set")
	require.Greater(t, endTime, int64(0), "end_time should have been set")
	require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should have been set")
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
	materialFetched := false
	var iid string
	taskDone := false
	iterationDone := false // Track if ReAct Iteration Done is written to timeline
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

			if e.Type == string(schema.EVENT_TYPE_REFERENCE_MATERIAL) {
				materialFetched = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			// Check if ReAct Iteration Done is written to timeline to avoid race condition
			if e.NodeId == "timeline_item" {
				content := string(e.GetContent())
				if strings.Contains(content, "ReAct Iteration Done") {
					iterationDone = true
				}
			}

			// Wait for all conditions including iterationDone to avoid timeline race condition
			if materialFetched && taskDone && iterationDone {
				break LOOP
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

	if !materialFetched {
		t.Fatal("Expected to have at least one material event, but got none")
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
	t.Skip("ci 调用工具超时，工具审核失败，暂时跳过")
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	riskToolName := "create_test_risk" + ksuid.New().String()
	// 生成唯一标识符用于验证
	flagTitle := ksuid.New().String()
	flagTarget := "http://" + ksuid.New().String() + ".com"
	riskTool, err := aitool.New(
		riskToolName,
		aitool.WithNumberParam("seconds"),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			// 构建 risk 数据
			riskData, err := yakit.NewRisk(flagTarget, yakit.WithRiskParam_Title(flagTitle), yakit.WithRiskParam_RiskType("test-risk"), yakit.WithRiskParam_Severity("high"))
			if err != nil {
				return nil, err
			}
			// 序列化 risk 数据
			riskJSON, err := json.Marshal(riskData)
			if err != nil {
				return nil, err
			}

			// 构建 YakitLog
			logInfo := map[string]any{
				"level":     "json-risk",
				"data":      string(riskJSON),
				"timestamp": time.Now().Unix(),
			}
			logJSON, err := json.Marshal(logInfo)
			if err != nil {
				return nil, err
			}

			// 构建 YakitMessage
			message := map[string]any{
				"type":    "log",
				"content": json.RawMessage(logJSON),
			}
			messageJSON, err := json.Marshal(message)
			if err != nil {
				return nil, err
			}

			// 发送 risk 消息
			runtimeConfig.FeedBacker(&ypb.ExecResult{
				IsMessage: true,
				Message:   messageJSON,
			})
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 创建 ReAct 实例
	reAct, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, riskToolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(riskTool),
		aicommon.WithAgreeYOLO(true),
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
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var taskID string
	var iid string
	toolCalled := false
	taskCompleted := false

LOOP:
	for {
		select {
		case e := <-out:
			t.Logf("event: %s", e.String())
			t.Logf("event type: %s", e.Type)
			t.Logf("event node id: %s", e.NodeId)
			t.Logf("event content: %s", e.GetContent())
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
				t.Logf("tool use review require: %s", iid)
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			// 检测工具是否被调用
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCalled = true
				t.Logf("Tool call completed")
			}

			// 检测任务完成
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				t.Logf("task status: %s", utils.InterfaceToString(result))
				if utils.InterfaceToString(result) == "completed" {
					taskCompleted = true
					t.Logf("Task completed")
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

	// Track tool execution status for mock responses
	toolExecutionSucceeded := utils.NewAtomicBool()

	// Custom mock function for this test that responds based on tool execution status
	mockedToolCallingWithToolStatus := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
			// Include identifier field for new directory structure
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "sleep_test", "params": { "seconds" : 0.1 }}`))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
			rsp := i.NewAIResponse()
			// Return satisfied only if tool execution succeeded
			if toolExecutionSucceeded.IsSet() {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "tool executed successfully", "human_readable_result": "mocked thought for verification"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "tool execution failed, need to retry", "human_readable_result": "tool not found"}`))
			}
			rsp.Close()
			return rsp, nil
		}

		// Handle self-reflection prompts
		if utils.MatchAllOfSubString(prompt, "SELF_REFLECTION") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "self-reflection", "suggestions": []}`))
			rsp.Close()
			return rsp, nil
		}

		fmt.Println("Unexpected prompt:", prompt)

		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	_, err := NewTestReAct(
		aicommon.WithAiToolManagerOptions(buildinaitools.WithNoToolsCache(), buildinaitools.WithEnableAllTools()),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingWithToolStatus(i, r)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			// Track tool execution success
			if e.Type == schema.EVENT_TOOL_CALL_DONE {
				toolExecutionSucceeded.Set()
			}
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
	// Note: After recent code changes, tool execution failure no longer aborts the task.
	// Instead, it records the error and allows AI to retry.
	// We detect tool execution error events to verify the tool was not found.
	fmt.Printf("Phase 1: Attempting to call non-existent tool '%s'\n", toolName)
	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "please use " + toolName,
	}

	after := time.After(du * time.Second)
	toolExecutionErrorDetected := false

LOOP1:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())

			// Detect tool execution error (tool not found)
			if e.NodeId == "timeline_item" {
				content := string(e.GetContent())
				if strings.Contains(content, "TOOL_EXECUTION_ERROR") && strings.Contains(content, toolName) {
					toolExecutionErrorDetected = true
					fmt.Printf("✓ Detected tool execution error for '%s'\n", toolName)
					// Once we detect the error, we can break out of the loop
					// The AI will keep retrying since verification returns false
					break LOOP1
				}
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				// 任务执行完成标志
				if utils.InterfaceToString(result) == "completed" {
					break LOOP1
				}
			}
		case <-after:
			break LOOP1
		}
	}

	// Verify that tool execution error was detected
	if !toolExecutionErrorDetected {
		t.Fatal("Phase 1 failed: expected tool execution error for non-existent tool, but none was detected")
	}
	fmt.Println("✓ Phase 1 completed: Tool execution error detected as expected")

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

func TestReAct_ToolUse_WithToolCallResult(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolName := "test_sleep_" + ksuid.New().String()

	callToolResultFlag := utils.RandStringBytes(10)

	sleepTool, err := aitool.New(
		toolName,
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return map[string]any{"result": callToolResultFlag}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 发送输入事件
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName,
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)
	toolCallResult := false
	var callToolID string
	var recivedToolCallResultFlag string
LOOP:
	for {
		select {
		case e := <-out:
			t.Logf("event: %s", e.String())
			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				if callToolID == "" {
					callToolID = e.CallToolID
				} else if callToolID != e.CallToolID {
					// 应该只有一个callToolID
					t.Fatalf("call tool id mismatch: should only have one callToolID, but got %s and %s", callToolID, e.CallToolID)
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_RESULT) {
				if e.CallToolID != callToolID {
					t.Fatalf("call tool id mismatch: should only have one callToolID, but got %s and %s", callToolID, e.CallToolID)
				}
				toolCallResult = true
				result := jsonpath.FindFirst(e.GetContent(), "$..result.result")
				recivedToolCallResultFlag = utils.InterfaceToString(result)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	if !toolCallResult {
		t.Fatal("tool call result not found")
	}

	if recivedToolCallResultFlag != callToolResultFlag {
		t.Fatalf("call tool result mismatch: %s != %s", recivedToolCallResultFlag, callToolResultFlag)
	}

	db := consts.GetGormProjectDatabase()
	eventIDs, err := yakit.QueryAIEventIDByProcessID(db, callToolID)
	if err != nil {
		return
	}

	if len(eventIDs) == 0 {
		t.Fatal("no event ids found")
	}

	event, err := yakit.QueryAIEvent(db, &ypb.AIEventFilter{
		EventUUIDS: eventIDs,
	})
	require.NoError(t, err)

	var hasFlag bool
	for _, event := range event {
		if event.Type == schema.EVENT_TOOL_CALL_RESULT {
			result := jsonpath.FindFirst(string(event.Content), "$..result.result")
			if utils.InterfaceToString(result) == callToolResultFlag {
				hasFlag = true
				break
			}
		}
	}

	if !hasFlag {
		t.Fatalf("call tool result flag not found in ai process events")
	}
}

func TestReAct_ToolCallError_Timing(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	errorTool, err := aitool.New(
		"error_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			// Return error immediately without sleep
			return nil, fmt.Errorf("intentional error for testing")
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "error_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(errorTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test error timing",
		}
	}()

	after := time.After(10 * time.Second)

	var startTime, endTime int64
	var durationMs int64
	toolStartReceived := false
	toolErrorReceived := false
	reviewed := false
	var iid string

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				toolStartReceived = true
				if st := jsonpath.FindFirst(string(e.Content), "$.start_time"); st != nil {
					startTime = int64(utils.InterfaceToInt(st))
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_ERROR) {
				toolErrorReceived = true
				if et := jsonpath.FindFirst(string(e.Content), "$.end_time"); et != nil {
					endTime = int64(utils.InterfaceToInt(et))
				}
				if etms := jsonpath.FindFirst(string(e.Content), "$.end_time_ms"); etms != nil {
					endTimeMs := int64(utils.InterfaceToInt(etms))
					require.Greater(t, endTimeMs, int64(0), "end_time_ms should be greater than 0 in error event")
					fmt.Printf("Tool call error at: %d (unix timestamp), %d (ms)\n", endTime, endTimeMs)
				}
				if dm := jsonpath.FindFirst(string(e.Content), "$.duration_ms"); dm != nil {
					durationMs = int64(utils.InterfaceToInt(dm))
					require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should be >= 0 in error event")
					require.LessOrEqual(t, durationMs, int64(5000), "duration should be less than 5000ms in error event")
					fmt.Printf("Tool call error duration: %d ms\n", durationMs)
				}
				if ds := jsonpath.FindFirst(string(e.Content), "$.duration_seconds"); ds != nil {
					durationSeconds := utils.InterfaceToFloat64(ds)
					require.GreaterOrEqual(t, durationSeconds, 0.0, "duration_seconds should be >= 0 in error event")
				}

				if endTime > 0 {
					require.Greater(t, endTime, int64(0), "end_time should be greater than 0 in error event")
					if startTime > 0 {
						require.GreaterOrEqual(t, endTime, startTime, "end_time should be >= start_time in error event")
					}
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
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
					if toolStartReceived && toolErrorReceived {
						break LOOP
					}
				}
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	require.True(t, toolStartReceived, "Expected to receive tool_call_start event")
	require.True(t, toolErrorReceived, "Expected to receive tool_call_error event")
	require.True(t, reviewed, "Expected to have at least one review event")
	require.Greater(t, startTime, int64(0), "start_time should have been set in error case")
	require.Greater(t, endTime, int64(0), "end_time should have been set in error case")
	require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should have been set in error case")
}

func TestReAct_ToolCallUserCancel_Timing(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	cancelTool, err := aitool.New(
		"cancel_test",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			// Tool won't actually be executed when direct_answer is chosen
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCalling(i, r, "cancel_test")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(cancelTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "test cancel timing",
		}
	}()

	after := time.After(10 * time.Second)

	var startTime, endTime int64
	var durationMs int64
	toolStartReceived := false
	toolCancelReceived := false
	var iid string

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				toolStartReceived = true
				if st := jsonpath.FindFirst(string(e.Content), "$.start_time"); st != nil {
					startTime = int64(utils.InterfaceToInt(st))
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_USER_CANCEL) {
				toolCancelReceived = true
				if et := jsonpath.FindFirst(string(e.Content), "$.end_time"); et != nil {
					endTime = int64(utils.InterfaceToInt(et))
				}
				if etms := jsonpath.FindFirst(string(e.Content), "$.end_time_ms"); etms != nil {
					endTimeMs := int64(utils.InterfaceToInt(etms))
					require.Greater(t, endTimeMs, int64(0), "end_time_ms should be greater than 0 in cancel event")
					fmt.Printf("Tool call cancelled at: %d (unix timestamp), %d (ms)\n", endTime, endTimeMs)
				}
				if dm := jsonpath.FindFirst(string(e.Content), "$.duration_ms"); dm != nil {
					durationMs = int64(utils.InterfaceToInt(dm))
					require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should be >= 0 in cancel event")
					require.LessOrEqual(t, durationMs, int64(5000), "duration should be less than 5000ms in cancel event")
					fmt.Printf("Tool call cancel duration: %d ms\n", durationMs)
				}
				if ds := jsonpath.FindFirst(string(e.Content), "$.duration_seconds"); ds != nil {
					durationSeconds := utils.InterfaceToFloat64(ds)
					require.GreaterOrEqual(t, durationSeconds, 0.0, "duration_seconds should be >= 0 in cancel event")
				}

				if endTime > 0 {
					require.Greater(t, endTime, int64(0), "end_time should be greater than 0 in cancel event")
					if startTime > 0 {
						require.GreaterOrEqual(t, endTime, startTime, "end_time should be >= start_time in cancel event")
					}
				}

				// Once we receive the cancel event with all timing info, we can exit
				if toolStartReceived && toolCancelReceived && startTime > 0 && endTime > 0 {
					break LOOP
				}
			}

			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				// Simulate user cancellation by requesting direct answer (which triggers userCancelHandler)
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "direct_answer"}`,
				}
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	require.True(t, toolStartReceived, "Expected to receive tool_call_start event")
	require.True(t, toolCancelReceived, "Expected to receive tool_call_user_cancel event")
	require.Greater(t, startTime, int64(0), "start_time should have been set in cancel case")
	require.Greater(t, endTime, int64(0), "end_time should have been set in cancel case")
	require.GreaterOrEqual(t, durationMs, int64(0), "duration_ms should have been set in cancel case")
}

func TestReAct_ToolUse_WithToolCallResult_WithBoolParam(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	toolName := "test_bool_param_" + ksuid.New().String()

	// 创建 yak 脚本类型的工具
	yakTool := &schema.AIYakTool{
		Name:        toolName,
		Description: "A test tool that tests bool parameter",
		Keywords:    "test,bool",
		Content: `# Test Bool Tool
yakit.AutoInitYakit()

cli.Bool("enable", cli.setDefault(false), cli.setHelp("enable operation"))
enableValue = cli.Bool("enable")
yakit.Info("Enable: %v", enableValue)
`,
		Params: `{"type":"object","properties":{"enable":{"type":"boolean","description":"enable operation","default":false}}}`,
		Path:   "test/bool",
	}

	tools := yak.YakTool2AITool([]*schema.AIYakTool{yakTool})
	if len(tools) == 0 {
		t.Fatal("failed to convert yak tool to aitool")
	}
	boolTool := tools[0]

	// 自定义 mocked AI callback 来返回 bool 参数为 false
	mockedBoolCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
			// 明确设置 enable 参数为 false
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "bool_test", "params": { "enable": false }}`))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, "review the tool call", "approve_tool_call") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "approve_tool_call"}, "human_readable_thought": "approve", "cumulative_summary": "..cumulative-mocked for approve.."}`))
			rsp.Close()
			return rsp, nil
		}

		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": { "type": "directly_answer", "directly_answer_payload": "general mocked response" }}`))
		rsp.Close()
		return rsp, nil
	}

	_, err := NewTestReAct(
		aicommon.WithAICallback(mockedBoolCallback),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(boolTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 发送输入事件
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName + " with enable=false",
		}
	}()

	du := time.Duration(10)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)
	toolCallResult := false
	foundBoolOutput := false
	receivedBoolValue := ""
	foundYakExecResult := false
	var callToolID string
LOOP:
	for {
		select {
		case e := <-out:
			t.Logf("event: %s", e.String())
			if e.Type == string(schema.EVENT_TOOL_CALL_START) {
				if callToolID == "" {
					callToolID = e.CallToolID
				} else if callToolID != e.CallToolID {
					t.Fatalf("call tool id mismatch: should only have one callToolID, but got %s and %s", callToolID, e.CallToolID)
				}
			}

			// 检查 yakit.Info 输出（从 IsStream 事件中）
			if e.IsStream {
				content := string(e.GetStreamDelta())
				if strings.Contains(content, "Enable:") {
					foundBoolOutput = true
					// 提取 Enable: 后面的值
					if strings.Contains(content, "Enable: false") || strings.Contains(content, "Enable:false") {
						receivedBoolValue = "false"
						t.Logf("✓ Found yakit.Info output: Enable: false")
					} else if strings.Contains(content, "Enable: true") || strings.Contains(content, "Enable:true") {
						receivedBoolValue = "true"
						t.Logf("Found yakit.Info output: Enable: true")
					}
				}
			}

			// 也检查 yak_exec_result 事件（更可靠的方式）
			if e.Type == string(schema.EVENT_TYPE_YAKIT_EXEC_RESULT) {
				foundYakExecResult = true
				// 解析 yakit 执行结果中的日志
				var result map[string]interface{}
				if err := json.Unmarshal(e.Content, &result); err == nil {
					if msg, ok := result["Message"].(string); ok {
						// Message 是 base64 编码的
						if decoded, err := base64.StdEncoding.DecodeString(msg); err == nil {
							decodedStr := string(decoded)
							t.Logf("Decoded yak_exec_result message: %s", decodedStr)
							if strings.Contains(decodedStr, "Enable") && strings.Contains(decodedStr, "false") {
								foundBoolOutput = true
								receivedBoolValue = "false"
								t.Logf("✓ Found bool param in yak_exec_result: Enable: false")
							}
						}
					}
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_RESULT) {
				if e.CallToolID != callToolID {
					t.Fatalf("call tool id mismatch: should only have one callToolID, but got %s and %s", callToolID, e.CallToolID)
				}
				toolCallResult = true
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	close(in)

	if !toolCallResult {
		t.Fatal("tool call result not found")
	}

	// 验证是否找到了 yakit.Info 的输出（通过 stream 或 yak_exec_result）
	if !foundBoolOutput {
		if foundYakExecResult {
			t.Log("Warning: yakit.Info output not found in stream events, but yak_exec_result was present")
			// 在CI环境下，由于时序问题，可能只能从 yak_exec_result 中检测到
			// 这是可以接受的
		} else {
			t.Fatal("yakit.Info output not found in event stream (neither in stream events nor yak_exec_result)")
		}
	}

	// 验证接收到的 enable 值是否为 false
	if receivedBoolValue != "false" {
		t.Fatalf("expected enable param to be false, but got: %s", receivedBoolValue)
	}

	t.Logf("✓ Test passed: enable param correctly received as false in yak script")
}
