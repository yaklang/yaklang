package aireact

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mockedToolCallingForFileEmit(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for tool calling file emit test", "cumulative_summary": "..cumulative-mocked for tool calling file emit test.."}
`, toolName)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		// Include identifier field for new directory structure
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "test_file_output", "params": { "message" : "test message", "output_lines": 5 }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "test-reason", "human_readable_result": "mocked thought for verification"}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolCall_FileEmit 测试工具调用时是否正确 emit 文件
func TestReAct_ToolCall_FileEmit(t *testing.T) {
	toolName := "test_file_emit_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 创建一个工具，会产生 stdout、stderr 和 result
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithNumberParam("output_lines"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default message")
			outputLines := int(params.GetInt("output_lines", 3))

			// 写入 stdout
			for i := 0; i < outputLines; i++ {
				fmt.Fprintf(stdout, "stdout line %d: %s\n", i+1, message)
			}

			// 写入 stderr
			fmt.Fprintf(stderr, "stderr: warning message\n")
			fmt.Fprintf(stderr, "stderr: error occurred\n")

			// 返回 result
			result := map[string]any{
				"status":    "success",
				"message":   message,
				"lines":     outputLines,
				"timestamp": time.Now().Unix(),
			}
			return result, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingForFileEmit(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true), // 自动同意，避免交互
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

	// 设置超时时间，确保测试在10s内完成
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	// 收集 emit 的 report markdown 文件路径
	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			// 检查文件 emit 事件 - 新格式只 emit 一个 .md report 文件
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
					log.Infof("Emitted report file: %s", filePath)
				}
			}

			// 检查工具调用完成
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			// 检查任务完成
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证 report 文件被 emit 且存在
	require.NotEmpty(t, reportFilePath, "report .md file should be emitted")
	require.True(t, utils.FileExists(reportFilePath), "report file should exist: %s", reportFilePath)

	// 读取并验证 report 内容
	reportContent, err := os.ReadFile(reportFilePath)
	require.NoError(t, err)
	contentStr := string(reportContent)
	log.Infof("Report content (first 500 chars): %s", utils.ShrinkString(contentStr, 500))

	// 验证 markdown 结构
	require.Contains(t, contentStr, "# Tool Call Report:")
	require.Contains(t, contentStr, "## Basic Info")
	require.Contains(t, contentStr, "## Parameters")
	require.Contains(t, contentStr, "## Execution Result")
	require.Contains(t, contentStr, "## STDOUT")
	require.Contains(t, contentStr, "## STDERR")

	// 验证参数内容 (YAML 格式)
	require.Contains(t, contentStr, "message")
	require.Contains(t, contentStr, "output_lines")

	// 验证 stdout 内容
	require.Contains(t, contentStr, "stdout line")

	// 验证 stderr 内容
	require.Contains(t, contentStr, "stderr")

	// 验证 result 内容
	require.Contains(t, contentStr, "success")

	// 验证文件名格式: {n}_{toolName}_{identifier}.md
	filename := filepath.Base(reportFilePath)
	require.True(t, strings.HasSuffix(filename, ".md"))
	require.Contains(t, filename, "test_file_output", "filename should contain identifier")

	// 验证路径结构: task_{index}/tool_calls/{n}_{tool}_{id}.md
	require.Contains(t, reportFilePath, "tool_calls")
	require.Contains(t, reportFilePath, "task_")

	// 验证文件名第一部分是数字
	parts := strings.Split(strings.TrimSuffix(filename, ".md"), "_")
	require.GreaterOrEqual(t, len(parts), 2, "filename should have format '{n}_{tool}_{id}.md'")
	require.True(t, utils.MatchAllOfRegexp(parts[0], `^\d+$`), "filename first part should be numeric, got: %s", parts[0])

	log.Infof("✓ Report file emitted successfully: %s", reportFilePath)

	// 清理
	defer func() {
		if utils.FileExists(reportFilePath) {
			os.Remove(reportFilePath)
		}
	}()
}

// TestReAct_ToolCall_FileEmit_LargeResult 测试大 result 时的文件 emit
func TestReAct_ToolCall_FileEmit_LargeResult(t *testing.T) {
	toolName := "test_large_result_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 创建一个工具，返回大的 result（5MB）
	testTool, err := aitool.New(
		toolName,
		aitool.WithNumberParam("size"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			size := int(params.GetInt("size", 5*1024*1024)) // 5MB

			// 生成大的 result（使用更高效的方式）
			largeData := strings.Repeat("A", size)
			result := map[string]any{
				"status": "success",
				"data":   largeData,
				"size":   size,
			}
			return result, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := r.GetPrompt()
			if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for large result test", "cumulative_summary": "..cumulative-mocked.."}
`, toolName)))
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
				rsp := i.NewAIResponse()
				// Include identifier field for new directory structure
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "generate_large_data", "params": { "size" : 5242880 }}`)) // 5MB
				rsp.Close()
				return rsp, nil
			}

			if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "test-reason", "human_readable_result": "mocked thought"}`))
				rsp.Close()
				return rsp, nil
			}

			return nil, utils.Errorf("unexpected prompt: %s", prompt)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 发送输入事件
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName + " with large result",
		}
	}()

	// 设置超时时间，确保测试在10s内完成
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}

	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证 report 文件存在且包含完整的大数据
	require.NotEmpty(t, reportFilePath, "report file should be emitted")
	require.True(t, utils.FileExists(reportFilePath), "report file should exist")

	content, err := os.ReadFile(reportFilePath)
	require.NoError(t, err)

	contentStr := string(content)
	// 验证文件包含完整的大数据（应该包含 "AAAA..."）
	require.Contains(t, contentStr, "AAAAAAAAAA", "report should contain large data")

	// 验证文件大小合理（应该接近 5MB，至少大于 4MB，有 markdown 开销）
	require.Greater(t, len(content), 4*1024*1024, "report file should be large (>= 4MB), but got %d bytes", len(content))

	log.Infof("✓ Large result report emitted successfully: %s (%d bytes)", reportFilePath, len(content))

	// 清理
	defer func() {
		if utils.FileExists(reportFilePath) {
			os.Remove(reportFilePath)
		}
	}()
}

// mockedToolCallingForEmptyOutput mocks AI responses for testing empty stdout/stderr
func mockedToolCallingForEmptyOutput(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for empty output test", "cumulative_summary": "..cumulative-mocked.."}
`, toolName)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		// Include identifier field for new directory structure
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "identifier": "empty_output_test", "params": { "message" : "test message" }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "test-reason", "human_readable_result": "mocked thought"}`))
		rsp.Close()
		return rsp, nil
	}

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolCall_FileEmit_EmptyStdoutStderr 测试空 stdout/stderr 时不创建文件
func TestReAct_ToolCall_FileEmit_EmptyStdoutStderr(t *testing.T) {
	toolName := "test_empty_output_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 创建一个工具，不产生 stdout 和 stderr，只返回 result
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default message")

			// 不写入 stdout 和 stderr，只返回 result
			result := map[string]any{
				"status":  "success",
				"message": message,
			}
			return result, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingForEmptyOutput(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
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

	// 设置超时时间
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	// 收集 emit 的 report 文件路径
	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
					log.Infof("Emitted report file: %s", filePath)
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证 report 文件存在
	require.NotEmpty(t, reportFilePath, "report file should be emitted")
	require.True(t, utils.FileExists(reportFilePath), "report file should exist")

	// 读取 report 内容
	reportContent, err := os.ReadFile(reportFilePath)
	require.NoError(t, err)
	contentStr := string(reportContent)

	// 验证 STDOUT 和 STDERR 部分标记为 (empty)
	// 在 markdown 中，空的 stdout/stderr 显示为 "(empty)"
	require.Contains(t, contentStr, "## STDOUT")
	require.Contains(t, contentStr, "## STDERR")

	// 验证仍然包含参数和结果
	require.Contains(t, contentStr, "## Parameters")
	require.Contains(t, contentStr, "## Execution Result")
	require.Contains(t, contentStr, "success")

	log.Infof("✓ Empty stdout/stderr test passed: report contains (empty) sections")

	// 清理
	defer func() {
		if utils.FileExists(reportFilePath) {
			os.Remove(reportFilePath)
		}
	}()
}

// mockedToolCallingWithCustomIdentifier mocks AI responses with a custom identifier for testing directory structure
func mockedToolCallingWithCustomIdentifier(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName, identifier string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for identifier test", "cumulative_summary": "..cumulative-mocked.."}
`, toolName)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		// Include the custom identifier
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action": "call-tool", "identifier": "%s", "params": { "message" : "test" }}`, identifier)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "test-reason", "human_readable_result": "mocked thought"}`))
		rsp.Close()
		return rsp, nil
	}

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolCall_FileEmit_WithIdentifier tests the new directory structure with identifier
func TestReAct_ToolCall_FileEmit_WithIdentifier(t *testing.T) {
	toolName := "test_identifier_" + ksuid.New().String()
	customIdentifier := "find_large_process"
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// Create a simple tool
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default")
			fmt.Fprintf(stdout, "Processing: %s\n", message)
			return map[string]any{"status": "success"}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingWithCustomIdentifier(i, r, toolName, customIdentifier)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Send input event
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName,
		}
	}()

	// Set timeout
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	// Collect emitted report file
	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
					log.Infof("Emitted report file: %s", filePath)
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// Verify report file exists
	require.NotEmpty(t, reportFilePath, "report file should be emitted")
	require.True(t, utils.FileExists(reportFilePath), "report file should exist")

	// Verify filename contains the identifier
	filename := filepath.Base(reportFilePath)
	require.Contains(t, filename, customIdentifier, "filename should contain identifier '%s', got: %s", customIdentifier, filename)

	// Verify path structure: task_{index}/tool_calls/{n}_{tool}_{identifier}.md
	require.Contains(t, reportFilePath, "task_")
	require.Contains(t, reportFilePath, "tool_calls")

	// Verify filename format: {number}_{toolname}_{identifier}.md
	nameWithoutExt := strings.TrimSuffix(filename, ".md")
	parts := strings.Split(nameWithoutExt, "_")
	require.GreaterOrEqual(t, len(parts), 3, "filename should have format '{n}_{tool}_{id}.md', got: %s", filename)
	require.True(t, utils.MatchAllOfRegexp(parts[0], `^\d+$`), "filename first part should be a number, got: %s", parts[0])

	log.Infof("✓ Identifier test passed: %s", reportFilePath)

	// Cleanup
	defer func() {
		if utils.FileExists(reportFilePath) {
			os.Remove(reportFilePath)
		}
	}()
}

// TestReAct_ToolCall_FileEmit_WithoutIdentifier tests directory structure when identifier is not provided
func TestReAct_ToolCall_FileEmit_WithoutIdentifier(t *testing.T) {
	toolName := "test_no_id_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// Create a simple tool
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return map[string]any{"status": "success"}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Mock without identifier
	mockedWithoutIdentifier := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked", "cumulative_summary": "..mocked.."}
`, toolName)))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
			rsp := i.NewAIResponse()
			// No identifier field - test fallback behavior
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test" }}`))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "ok", "human_readable_result": "mocked"}`))
			rsp.Close()
			return rsp, nil
		}

		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(mockedWithoutIdentifier),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Send input event
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName,
		}
	}()

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// Verify report file exists
	require.NotEmpty(t, reportFilePath, "report file should be emitted")
	require.True(t, utils.FileExists(reportFilePath), "report file should exist")

	// Verify filename format: {number}_{toolname}.md (without identifier)
	filename := filepath.Base(reportFilePath)
	require.True(t, strings.HasSuffix(filename, ".md"))

	// Verify path structure
	require.Contains(t, reportFilePath, "task_")
	require.Contains(t, reportFilePath, "tool_calls")

	nameWithoutExt := strings.TrimSuffix(filename, ".md")
	parts := strings.Split(nameWithoutExt, "_")
	require.GreaterOrEqual(t, len(parts), 2, "filename should have format '{n}_{tool}.md', got: %s", filename)
	require.True(t, utils.MatchAllOfRegexp(parts[0], `^\d+$`), "filename first part should be a number, got: %s", parts[0])

	log.Infof("✓ No-identifier test passed: %s", reportFilePath)

	// Cleanup
	defer func() {
		if utils.FileExists(reportFilePath) {
			os.Remove(reportFilePath)
		}
	}()
}

// TestReAct_ToolCall_LogDir tests tool_call_log_dir event is emitted and matches pinned file locations.
func TestReAct_ToolCall_LogDir(t *testing.T) {
	toolName := "test_tool_call_log_dir_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithNumberParam("output_lines"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default message")
			outputLines := int(params.GetInt("output_lines", 3))

			for i := 0; i < outputLines; i++ {
				fmt.Fprintf(stdout, "stdout line %d: %s\n", i+1, message)
			}
			fmt.Fprintf(stderr, "stderr: test\n")

			return map[string]any{
				"status":  "success",
				"message": message,
				"lines":   outputLines,
			}, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingForFileEmit(i, r, toolName)
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(testTool),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "please use " + toolName,
		}
	}()

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var toolCallLogPath string // now a file path (.md) instead of a directory
	var toolCallLogPathEventCallToolID string
	var callToolID string
	var reportFilePath string
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == schema.EVENT_TOOL_CALL_START {
				callToolID = utils.InterfaceToString(jsonpath.FindFirst(string(e.GetContent()), "$.call_tool_id"))
				log.Infof("Tool call started. call_tool_id=%s", callToolID)
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_LOG_DIR) {
				content := string(e.GetContent())
				toolCallLogPath = utils.InterfaceToString(jsonpath.FindFirst(content, "$.dir_path"))
				toolCallLogPathEventCallToolID = utils.InterfaceToString(jsonpath.FindFirst(content, "$.call_tool_id"))
			}

			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.HasSuffix(filePath, ".md") && strings.Contains(filePath, "tool_calls") {
					reportFilePath = filePath
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallDone = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallDone && taskDone {
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// Verify tool_call_log_dir event emitted (now contains file path)
	require.NotEmpty(t, toolCallLogPath, "Expected tool_call_log_dir event with dir_path")
	require.Equal(t, callToolID, toolCallLogPathEventCallToolID)

	// Verify the emitted path is a .md file that exists
	require.True(t, strings.HasSuffix(toolCallLogPath, ".md"), "log path should be a .md file, got: %s", toolCallLogPath)
	require.True(t, utils.FileExists(toolCallLogPath), "report file should exist: %s", toolCallLogPath)

	// Verify path structure
	require.Contains(t, toolCallLogPath, "tool_calls")
	require.Contains(t, toolCallLogPath, "task_")
	require.Contains(t, filepath.Base(toolCallLogPath), "test_file_output", "filename should contain identifier")

	// Verify pinned file matches the log path
	require.NotEmpty(t, reportFilePath, "report file should be pinned")
	require.Equal(t, toolCallLogPath, reportFilePath, "pinned file should match log path")

	defer func() {
		if utils.FileExists(toolCallLogPath) {
			os.Remove(toolCallLogPath)
		}
	}()
}
