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

	// 收集 emit 的文件路径
	emittedFiles := make(map[string]string) // type -> filepath
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			// 检查文件 emit 事件
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					// 根据文件名判断类型
					filename := filepath.Base(filePath)
					if strings.Contains(filename, "params") {
						emittedFiles["params"] = filePath
					} else if strings.Contains(filename, "stdout") {
						emittedFiles["stdout"] = filePath
					} else if strings.Contains(filename, "stderr") {
						emittedFiles["stderr"] = filePath
					} else if strings.Contains(filename, "result") {
						emittedFiles["result"] = filePath
					}
					log.Infof("Emitted file: %s (type: %s)", filePath, filename)
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
				// 等待一下，确保所有文件都被 emit
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// 验证工具调用完成
	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}

	// 验证任务完成
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证所有文件都被 emit
	expectedFiles := []string{"params", "stdout", "stderr", "result"}
	for _, fileType := range expectedFiles {
		if filePath, ok := emittedFiles[fileType]; !ok {
			t.Errorf("Expected %s file to be emitted, but it was not", fileType)
		} else {
			// 验证文件存在
			if !utils.FileExists(filePath) {
				t.Errorf("Emitted %s file does not exist: %s", fileType, filePath)
			} else {
				// 验证文件内容
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Errorf("Failed to read %s file: %v", fileType, err)
				} else {
					contentStr := string(content)
					log.Infof("%s file content (first 200 chars): %s", fileType, utils.ShrinkString(contentStr, 200))

					// 验证文件内容不为空（除了可能为空的 stderr）
					if fileType != "stderr" && len(contentStr) == 0 {
						t.Errorf("%s file is empty", fileType)
					}

					// 验证特定内容
					switch fileType {
					case "params":
						if !strings.Contains(contentStr, "message") || !strings.Contains(contentStr, "output_lines") {
							t.Errorf("params file should contain 'message' and 'output_lines', got: %s", utils.ShrinkString(contentStr, 100))
						}
					case "stdout":
						if !strings.Contains(contentStr, "stdout line") {
							t.Errorf("stdout file should contain 'stdout line', got: %s", utils.ShrinkString(contentStr, 100))
						}
					case "stderr":
						// stderr 可能为空，但如果存在应该包含错误信息
						if len(contentStr) > 0 && !strings.Contains(contentStr, "stderr") {
							t.Errorf("stderr file should contain 'stderr', got: %s", utils.ShrinkString(contentStr, 100))
						}
					case "result":
						if !strings.Contains(contentStr, "status") || !strings.Contains(contentStr, "success") {
							t.Errorf("result file should contain 'status' and 'success', got: %s", utils.ShrinkString(contentStr, 100))
						}
					}
				}
			}
		}
	}

	log.Infof("✓ All files emitted successfully: %v", emittedFiles)

	// 验证新的目录结构
	// 新目录结构：task_{{task_index}}/tool_calls/{{index}}_{{tool-name}}_{{identifier}}/{{type}}.txt
	// 例如：task_1-1/tool_calls/1_test_file_emit_xxx_test_file_output/params.txt
	for fileType, filePath := range emittedFiles {
		// 验证文件名是简单的类型名
		filename := filepath.Base(filePath)
		expectedFilename := fileType + ".txt"
		if filename != expectedFilename {
			t.Errorf("File %s should have filename '%s', got: %s", fileType, expectedFilename, filename)
		}

		// 验证目录结构包含 tool_calls
		if !strings.Contains(filePath, "tool_calls") {
			t.Errorf("File path for %s should contain 'tool_calls', got: %s", fileType, filePath)
		}

		// 验证目录结构包含 task_ 前缀
		if !strings.Contains(filePath, "task_") {
			t.Errorf("File path for %s should contain 'task_' prefix, got: %s", fileType, filePath)
		}

		// 验证目录包含 identifier (test_file_output)
		dirName := filepath.Base(filepath.Dir(filePath))
		if !strings.Contains(dirName, "test_file_output") {
			t.Errorf("Directory name for %s should contain 'test_file_output' identifier, got: %s", fileType, dirName)
		}

		// 验证目录名格式: {{index}}_{{tool-name}}_{{identifier}}
		// 例如: 1_test_file_emit_xxx_test_file_output
		parts := strings.Split(dirName, "_")
		if len(parts) < 3 {
			t.Errorf("Directory name for %s should have format '{{index}}_{{tool-name}}_{{identifier}}', got: %s", fileType, dirName)
		} else {
			// 第一部分应该是数字
			if !utils.MatchAllOfRegexp(parts[0], `^\d+$`) {
				t.Errorf("Directory name first part should be numeric, got: %s", parts[0])
			}
			log.Infof("✓ File %s has correct directory structure: %s", fileType, dirName)
		}
	}

	// 清理：删除测试产生的临时文件
	defer func() {
		for fileType, filePath := range emittedFiles {
			if utils.FileExists(filePath) {
				if err := os.Remove(filePath); err != nil {
					log.Warnf("Failed to remove test file %s (%s): %v", fileType, filePath, err)
				} else {
					log.Infof("Cleaned up test file: %s", filePath)
				}
			}
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

	emittedFiles := make(map[string]string)
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					filename := filepath.Base(filePath)
					if strings.Contains(filename, "params") {
						emittedFiles["params"] = filePath
					} else if strings.Contains(filename, "stdout") {
						emittedFiles["stdout"] = filePath
					} else if strings.Contains(filename, "stderr") {
						emittedFiles["stderr"] = filePath
					} else if strings.Contains(filename, "result") {
						emittedFiles["result"] = filePath
					}
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

	// 验证 result 文件存在且包含完整内容
	if resultPath, ok := emittedFiles["result"]; ok {
		if !utils.FileExists(resultPath) {
			t.Fatal("Result file does not exist")
		}

		content, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("Failed to read result file: %v", err)
		}

		// 验证文件包含完整的大数据（应该包含 "AAAA..."）
		contentStr := string(content)
		if !strings.Contains(contentStr, "AAAAAAAAAA") {
			t.Errorf("Result file should contain large data, got: %s", utils.ShrinkString(contentStr, 200))
		}

		// 验证文件大小合理（应该接近 5MB，至少大于 4MB）
		if len(content) < 4*1024*1024 {
			t.Errorf("Result file should be large (>= 4MB), but got %d bytes", len(content))
		}

		log.Infof("✓ Large result file emitted successfully: %s (%d bytes)", resultPath, len(content))
	} else {
		t.Fatal("Result file was not emitted")
	}

	log.Infof("✓ Large result test completed successfully")

	// 清理：删除测试产生的临时文件
	defer func() {
		for fileType, filePath := range emittedFiles {
			if utils.FileExists(filePath) {
				if err := os.Remove(filePath); err != nil {
					log.Warnf("Failed to remove test file %s (%s): %v", fileType, filePath, err)
				} else {
					log.Infof("Cleaned up test file: %s", filePath)
				}
			}
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

	// 收集 emit 的文件路径
	emittedFiles := make(map[string]string)
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					filename := filepath.Base(filePath)
					if strings.Contains(filename, "params") {
						emittedFiles["params"] = filePath
					} else if strings.Contains(filename, "stdout") {
						emittedFiles["stdout"] = filePath
					} else if strings.Contains(filename, "stderr") {
						emittedFiles["stderr"] = filePath
					} else if strings.Contains(filename, "result") {
						emittedFiles["result"] = filePath
					}
					log.Infof("Emitted file: %s (type: %s)", filePath, filename)
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

	// 验证工具调用完成
	if !toolCallDone {
		t.Fatal("Tool call was not completed")
	}

	// 验证任务完成
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证 params 和 result 文件存在
	if _, ok := emittedFiles["params"]; !ok {
		t.Error("Expected params file to be emitted, but it was not")
	}
	if _, ok := emittedFiles["result"]; !ok {
		t.Error("Expected result file to be emitted, but it was not")
	}

	// 验证 stdout 和 stderr 文件不存在（因为没有输出）
	if _, ok := emittedFiles["stdout"]; ok {
		t.Error("stdout file should NOT be emitted when stdout is empty")
	}
	if _, ok := emittedFiles["stderr"]; ok {
		t.Error("stderr file should NOT be emitted when stderr is empty")
	}

	log.Infof("✓ Empty stdout/stderr test passed: only params and result files emitted")

	// 清理：删除测试产生的临时文件
	defer func() {
		for fileType, filePath := range emittedFiles {
			if utils.FileExists(filePath) {
				if err := os.Remove(filePath); err != nil {
					log.Warnf("Failed to remove test file %s (%s): %v", fileType, filePath, err)
				} else {
					log.Infof("Cleaned up test file: %s", filePath)
				}
			}
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

	// Collect emitted files
	emittedFiles := make(map[string]string)
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					filename := filepath.Base(filePath)
					// New format: params.txt, stdout.txt, stderr.txt, result.txt
					switch filename {
					case "params.txt":
						emittedFiles["params"] = filePath
					case "stdout.txt":
						emittedFiles["stdout"] = filePath
					case "stderr.txt":
						emittedFiles["stderr"] = filePath
					case "result.txt":
						emittedFiles["result"] = filePath
					}
					log.Infof("Emitted file: %s", filePath)
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

	// Verify directory structure contains the identifier
	for fileType, filePath := range emittedFiles {
		// 验证目录包含 identifier
		dirPath := filepath.Dir(filePath)
		dirName := filepath.Base(dirPath)

		// 目录名应该包含 identifier (find_large_process)
		if !strings.Contains(dirName, customIdentifier) {
			t.Errorf("Directory name for %s should contain identifier '%s', got: %s", fileType, customIdentifier, dirName)
		}

		// 验证目录结构: task_{index}/tool_calls/{number}_{tool}_{identifier}
		if !strings.Contains(filePath, "task_") {
			t.Errorf("File path should contain 'task_' prefix, got: %s", filePath)
		}
		if !strings.Contains(filePath, "tool_calls") {
			t.Errorf("File path should contain 'tool_calls', got: %s", filePath)
		}

		// 验证目录名格式: {number}_{toolname}_{identifier}
		// 例如: 1_test_identifier_xxx_find_large_process
		parts := strings.Split(dirName, "_")
		if len(parts) < 3 {
			t.Errorf("Directory should have format '{number}_{tool}_{identifier}', got: %s", dirName)
		} else {
			// 验证第一部分是数字
			if !utils.MatchAllOfRegexp(parts[0], `^\d+$`) {
				t.Errorf("Directory first part should be a number, got: %s (in %s)", parts[0], dirName)
			}
		}

		log.Infof("✓ %s file has correct directory structure: %s", fileType, filePath)
	}

	// Verify at least params and result files exist
	if _, ok := emittedFiles["params"]; !ok {
		t.Error("Expected params file to be emitted")
	}
	if _, ok := emittedFiles["result"]; !ok {
		t.Error("Expected result file to be emitted")
	}

	log.Infof("✓ Identifier directory structure test passed: %v", emittedFiles)

	// Cleanup
	defer func() {
		for _, filePath := range emittedFiles {
			if utils.FileExists(filePath) {
				// Also try to remove parent directories
				dirPath := filepath.Dir(filePath)
				os.RemoveAll(dirPath)
			}
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

	emittedFiles := make(map[string]string)
	toolCallDone := false
	taskDone := false

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					filename := filepath.Base(filePath)
					switch filename {
					case "params.txt":
						emittedFiles["params"] = filePath
					case "result.txt":
						emittedFiles["result"] = filePath
					}
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

	// Verify directory structure without identifier
	for fileType, filePath := range emittedFiles {
		dirPath := filepath.Dir(filePath)
		dirName := filepath.Base(dirPath)

		// 目录名格式应为: {number}_{toolname} (没有 identifier)
		// 例如: 1_test_no_id_xxx
		if !strings.Contains(filePath, "task_") {
			t.Errorf("File path should contain 'task_' prefix, got: %s", filePath)
		}
		if !strings.Contains(filePath, "tool_calls") {
			t.Errorf("File path should contain 'tool_calls', got: %s", filePath)
		}

		parts := strings.Split(dirName, "_")
		if len(parts) < 2 {
			t.Errorf("Directory should have format '{number}_{tool}', got: %s", dirName)
		} else {
			if !utils.MatchAllOfRegexp(parts[0], `^\d+$`) {
				t.Errorf("Directory first part should be a number, got: %s", parts[0])
			}
		}

		log.Infof("✓ %s file without identifier has correct structure: %s", fileType, filePath)
	}

	log.Infof("✓ No-identifier directory structure test passed")

	// Cleanup
	defer func() {
		for _, filePath := range emittedFiles {
			if utils.FileExists(filePath) {
				dirPath := filepath.Dir(filePath)
				os.RemoveAll(dirPath)
			}
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

	var toolCallLogDir string
	var toolCallLogDirEventCallToolID string
	var callToolID string
	pinnedFiles := make([]string, 0, 4)
	pinnedByName := make(map[string]string)
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
				toolCallLogDir = utils.InterfaceToString(jsonpath.FindFirst(content, "$.dir_path"))
				toolCallLogDirEventCallToolID = utils.InterfaceToString(jsonpath.FindFirst(content, "$.call_tool_id"))
			}

			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.GetContent())
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" {
					switch filepath.Base(filePath) {
					case "params.txt", "stdout.txt", "stderr.txt", "result.txt":
						pinnedFiles = append(pinnedFiles, filePath)
						pinnedByName[filepath.Base(filePath)] = filePath
					}
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

	if toolCallLogDir == "" {
		t.Fatal("Expected tool_call_log_dir event with dir_path, but got empty")
	}

	require.Equal(t, callToolID, toolCallLogDirEventCallToolID)

	info, err := os.Stat(toolCallLogDir)
	if err != nil {
		t.Fatalf("Tool call log dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("Tool call log dir is not a directory: %s", toolCallLogDir)
	}

	if !strings.Contains(toolCallLogDir, "tool_calls") || !strings.Contains(toolCallLogDir, "task_") {
		t.Fatalf("Tool call log dir path should contain 'task_' and 'tool_calls', got: %s", toolCallLogDir)
	}
	if !strings.Contains(filepath.Base(toolCallLogDir), "test_file_output") {
		t.Fatalf("Tool call log dir name should contain identifier 'test_file_output', got: %s", filepath.Base(toolCallLogDir))
	}

	if len(pinnedFiles) == 0 {
		t.Fatal("Expected at least one pinned tool-call file (params/stdout/stderr/result), got none")
	}
	if pinnedByName["params.txt"] == "" || pinnedByName["result.txt"] == "" {
		t.Fatalf("Expected at least params.txt and result.txt to be pinned, got: %v", pinnedByName)
	}
	for _, pinned := range pinnedFiles {
		rel, err := filepath.Rel(toolCallLogDir, pinned)
		if err != nil {
			t.Fatalf("Failed to build relative path from tool call log dir: %v", err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			t.Fatalf("Pinned file should be under tool call log dir. dir=%s file=%s", toolCallLogDir, pinned)
		}
	}

	defer func() {
		// Safety guard: only remove directories that match the expected structure.
		if strings.Contains(toolCallLogDir, "tool_calls") && strings.Contains(toolCallLogDir, "task_") {
			_ = os.RemoveAll(toolCallLogDir)
		}
	}()
}
