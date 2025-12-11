package aireact

import (
	"bytes"
	"fmt"
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
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test message", "output_lines": 5 }}`))
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

	// 验证文件名格式为 taskIndex_number (例如: 1-1_1)
	for fileType, filePath := range emittedFiles {
		filename := filepath.Base(filePath)
		// 文件名格式应该是: tool-call-{toolname}-{type}-{taskIndex_number}.txt
		// 例如: tool-call-test_file_emit_xxx-params-1-1_1.txt
		if !strings.Contains(filename, "_") {
			t.Errorf("File %s should contain '_' in index part, got: %s", fileType, filename)
		} else {
			// 提取 index 部分（最后一个 - 之后，.txt 之前）
			parts := strings.Split(filename, "-")
			if len(parts) >= 4 {
				indexPart := strings.TrimSuffix(parts[len(parts)-1], ".txt")
				// indexPart 应该是 taskIndex_number 格式
				if !strings.Contains(indexPart, "_") {
					t.Errorf("File %s index should be in format 'taskIndex_number', got: %s", fileType, indexPart)
				} else {
					indexParts := strings.Split(indexPart, "_")
					if len(indexParts) != 2 {
						t.Errorf("File %s index should have format 'taskIndex_number', got: %s", fileType, indexPart)
					} else {
						// 验证 number 部分是数字
						if !utils.MatchAllOfRegexp(indexParts[1], `^\d+$`) {
							t.Errorf("File %s index number part should be numeric, got: %s", fileType, indexParts[1])
						}
						log.Infof("✓ File %s has correct index format: %s", fileType, indexPart)
					}
				}
			}
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
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "size" : 5242880 }}`)) // 5MB
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
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "message" : "test message" }}`))
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

