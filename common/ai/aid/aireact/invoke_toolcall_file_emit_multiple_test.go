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

func mockedToolCallingMultiple(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string, callCount *int) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "%s" },
"human_readable_thought": "mocked thought for multiple tool calls test", "cumulative_summary": "..cumulative-mocked.."}
`, toolName)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		*callCount++
		message := fmt.Sprintf("call %d", *callCount)
		identifier := fmt.Sprintf("call_%d_test", *callCount)
		rsp := i.NewAIResponse()
		// Include identifier field for new directory structure
		rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action": "call-tool", "identifier": "%s", "params": { "message" : "%s", "output_lines": 2 }}`, identifier, message)))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		// 如果调用次数少于2次，继续调用工具
		if *callCount < 2 {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "need more calls", "human_readable_result": "need to call tool again"}`))
		} else {
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "done", "human_readable_result": "completed"}`))
		}
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

// TestReAct_ToolCall_FileEmit_Multiple 测试多次工具调用时的文件命名格式
func TestReAct_ToolCall_FileEmit_Multiple(t *testing.T) {
	toolName := "test_multiple_" + ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	callCount := 0

	// 创建一个工具
	testTool, err := aitool.New(
		toolName,
		aitool.WithStringParam("message"),
		aitool.WithNumberParam("output_lines"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			message := params.GetString("message", "default message")
			outputLines := int(params.GetInt("output_lines", 2))

			// 写入 stdout
			for i := 0; i < outputLines; i++ {
				fmt.Fprintf(stdout, "stdout line %d: %s\n", i+1, message)
			}

			// 返回 result
			result := map[string]any{
				"status":  "success",
				"message": message,
				"call":    callCount,
			}
			return result, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingMultiple(i, r, toolName, &callCount)
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
			FreeInput:   "please use " + toolName + " twice",
		}
	}()

	// 设置超时时间
	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	// 收集所有 emit 的文件路径，按类型和调用次数分组
	emittedFilesByCall := make(map[int]map[string]string) // callNumber -> type -> filepath
	toolCallCount := 0
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
					filename := filepath.Base(filePath)
					// 新目录结构: task_{task_index}/tool_calls/{number}_{tool}_{identifier}/params.txt
					// 从目录名中提取调用次数
					dirName := filepath.Base(filepath.Dir(filePath))
					parts := strings.Split(dirName, "_")
					if len(parts) >= 2 {
						// 第一部分是调用次数
						callNumber := utils.InterfaceToInt(parts[0])
						if callNumber > 0 {
							if emittedFilesByCall[callNumber] == nil {
								emittedFilesByCall[callNumber] = make(map[string]string)
							}
							// 新格式: 文件名是简单的 params.txt, stdout.txt, stderr.txt, result.txt
							switch filename {
							case "params.txt":
								emittedFilesByCall[callNumber]["params"] = filePath
							case "stdout.txt":
								emittedFilesByCall[callNumber]["stdout"] = filePath
							case "stderr.txt":
								emittedFilesByCall[callNumber]["stderr"] = filePath
							case "result.txt":
								emittedFilesByCall[callNumber]["result"] = filePath
							}
							log.Infof("Emitted file for call %d: %s (dir: %s)", callNumber, filename, dirName)
						}
					}
				}
			}

			// 检查工具调用完成
			if e.Type == string(schema.EVENT_TOOL_CALL_DONE) {
				toolCallCount++
			}

			// 检查任务完成
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					taskDone = true
				}
			}

			if toolCallCount >= 2 && taskDone {
				// 等待一下，确保所有文件都被 emit
				time.Sleep(500 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// 验证至少调用了2次工具
	if toolCallCount < 2 {
		t.Fatalf("Expected at least 2 tool calls, but got %d", toolCallCount)
	}

	// 验证任务完成
	if !taskDone {
		t.Fatal("Task was not completed")
	}

	// 验证每次工具调用都有对应的文件
	expectedFiles := []string{"params", "stdout", "result"}
	for callNumber := 1; callNumber <= 2; callNumber++ {
		files, ok := emittedFilesByCall[callNumber]
		if !ok {
			t.Errorf("Expected files for call %d, but none were found", callNumber)
			continue
		}

		for _, fileType := range expectedFiles {
			if filePath, ok := files[fileType]; !ok {
				t.Errorf("Expected %s file for call %d, but it was not emitted", fileType, callNumber)
			} else {
				// 验证文件存在
				if !utils.FileExists(filePath) {
					t.Errorf("File %s for call %d does not exist: %s", fileType, callNumber, filePath)
				} else {
					// 验证新的目录结构
					// 目录名格式: {number}_{tool}_{identifier}
					dirName := filepath.Base(filepath.Dir(filePath))
					if !strings.HasPrefix(dirName, fmt.Sprintf("%d_", callNumber)) {
						t.Errorf("Directory for call %d should start with '%d_', got: %s", callNumber, callNumber, dirName)
					}
					// 验证目录名包含 identifier
					if !strings.Contains(dirName, fmt.Sprintf("call_%d_test", callNumber)) {
						t.Errorf("Directory for call %d should contain identifier 'call_%d_test', got: %s", callNumber, callNumber, dirName)
					}
					log.Infof("✓ Call %d %s file: %s (in dir: %s)", callNumber, fileType, filepath.Base(filePath), dirName)
				}
			}
		}
	}

	log.Infof("✓ Multiple tool calls test completed successfully: %d calls, files: %v", toolCallCount, emittedFilesByCall)

	// 清理：删除测试产生的临时目录
	defer func() {
		deletedDirs := make(map[string]bool)
		for _, files := range emittedFilesByCall {
			for _, filePath := range files {
				dirPath := filepath.Dir(filePath)
				if !deletedDirs[dirPath] {
					if err := os.RemoveAll(dirPath); err != nil {
						log.Warnf("Failed to remove test directory %s: %v", dirPath, err)
					} else {
						log.Infof("Cleaned up test directory: %s", dirPath)
					}
					deletedDirs[dirPath] = true
				}
			}
		}
	}()
}
